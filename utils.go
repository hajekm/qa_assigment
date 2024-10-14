package bloomreachQaAssignment

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"golang.org/x/net/html"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
)

const (
	customerExpUrl = "https://gqa.api.gdev.exponea.com/data/v2/projects/6f521150-d92d-11ed-a284-de49e5a76b0b/customers/export-one"
	apiKey         = "fbiiqlwwf5gnnuxns77fkxfkoysnl6f0nhf21xixykxxsiia9c341x4ewr4w30nb"
	apiSecret      = "fypwdlyoi2sou87lvmtl80nmf68y6hxfngigssul62ojsf2qxmneiag3xo4wnpfg"
	customerID     = "customer-7d3445"
)

type CustomerExport struct {
	Created              int      `json:"created"`
	Errors               []string `json:"errors"`
	ExportArchivedEvents bool     `json:"export_archived_events"`
	IDs                  struct {
		Cookie          []string `json:"cookie"`
		GoogleAnalytics []string `json:"google_analytics"`
		Registered      string   `json:"registered"`
	} `json:"ids"`
	Properties struct {
		SurveyLink string `json:"survey link"`
	} `json:"properties"`
	Success bool    `json:"success"`
	Events  []Event `json:"events"`
}

type Event struct {
	Properties struct {
		Answer        interface{} `json:"answer"`
		Question      string      `json:"question"`
		QuestionId    int         `json:"question_id"`
		QuestionIndex int         `json:"question_index"`
		SurveyId      string      `json:"survey_id"`
		SurveyName    string      `json:"survey_name"`
	} `json:"properties"`
	Timestamp float64 `json:"timestamp"`
	Type      string  `json:"type"`
}

func postCustomerExport() (CustomerExport, error) {
	jsonData := []byte(fmt.Sprintf("{\"customer_ids\": {\"registered\": \"%s\"}}", customerID))

	req, err := http.NewRequest("POST", customerExpUrl, bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Println("Error creating request:", err)
		return CustomerExport{}, err
	}

	base64Encoded := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", apiKey, apiSecret)))

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Basic %s", base64Encoded))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error sending request:", err)
		return CustomerExport{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response:", err)
		return CustomerExport{}, err
	}
	fmt.Println("Response:", string(body))
	var ce CustomerExport
	err = json.Unmarshal(body, &ce)
	if err != nil {
		fmt.Println("Error reading response:", err)
		return CustomerExport{}, err
	}

	return ce, nil
}

func getSurveyForm(surveyUrl string) (*html.Node, []*http.Cookie, error) {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar,
	}
	resp, err := client.Get(surveyUrl)
	if err != nil {
		fmt.Println("Error:", err)
		return nil, nil, err
	}
	defer resp.Body.Close()
	cookies := client.Jar.Cookies(resp.Request.URL)
	doc, err := html.Parse(resp.Body)
	if err != nil {
		fmt.Println("Error:", err)
		return nil, nil, err
	}
	return doc, cookies, nil
}

func submitSurvey(dest string, formData url.Values, cookies []*http.Cookie) (int, error) {
	req, err := http.NewRequest("POST", dest, strings.NewReader(formData.Encode()))
	if err != nil {
		fmt.Println("Error creating request:", err)
		return 0, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error sending request:", err)
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

type Survey struct {
	Questions  map[string]Question `json:"questions"`
	SurveyName string              `json:"survey_name"`
	CsrfToken  string              `json:"csrf_token"`
	Url        string              `json:"url"`
}

type Question struct {
	Question      string       `json:"question"`
	Required      bool         `json:"required"`
	QuestionID    int          `json:"question_id"`
	QuestionIndex int          `json:"question_index"`
	QuestionName  string       `json:"question_name"`
	Properties    []Properties `json:"properties"`
}

type Properties struct {
	PropertyIndex int    `json:"property_index"`
	Value         string `json:"value"`
	Type          string `json:"type"`
}

func extractSurvey(doc *html.Node) (Survey, error) {
	survey := Survey{Questions: make(map[string]Question)}

	var visitNode func(*html.Node)
	visitNode = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "form" {
			for _, attr := range n.Attr {
				if attr.Key == "action" {
					survey.Url = fmt.Sprintf("https://gqa.cdn.gdev.exponea.com%s", attr.Val)
				}
			}
		}
		if n.Type == html.ElementNode && n.Data == "div" {
			for _, attr := range n.Attr {
				if attr.Key == "class" && strings.Contains(attr.Val, "question-wrapper") {
					exQuestion := extractQuestion(n)
					survey.Questions[exQuestion.QuestionName] = exQuestion
				}
			}
		}
		if n.Type == html.ElementNode && n.Data == "h1" {
			survey.SurveyName = getTextContent(n)
		}
		if n.Type == html.ElementNode && n.Data == "input" {
			tokenMarker := false
			for i := 0; i < len(n.Attr); i++ {
				if n.Attr[i].Key == "id" && n.Attr[i].Val == "csrf_token" && !tokenMarker {
					tokenMarker = true
					i = 0
				}
				if n.Attr[i].Key == "value" && tokenMarker {
					survey.CsrfToken = n.Attr[i].Val
					break
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			visitNode(child)
		}

	}
	visitNode(doc)

	return survey, nil
}

func extractQuestion(n *html.Node) Question {
	q := Question{}
	var props []Properties
	var visitNode func(*html.Node)
	visitNode = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "input":
				var p Properties
				for _, attr := range n.Attr {
					switch attr.Key {
					case "id":
						parts := strings.Split(attr.Val, "-")
						idx, err := strconv.Atoi(parts[len(parts)-1])
						if err != nil {
							fmt.Println("Error parsing property id:", err)
							idx = 0
						}
						p.PropertyIndex = idx
					case "type":
						p.Type = attr.Val
					case "value":
						p.Value = attr.Val
					case "name":
						parts := strings.Split(attr.Val, "-")
						idx, err := strconv.Atoi(parts[len(parts)-1])
						if err != nil {
							fmt.Println("Error parsing question id:", err)
							idx = 0
						}
						q.QuestionIndex = idx
						q.QuestionID = idx
						q.QuestionName = attr.Val
					}
				}
				props = append(props, p)
			case "h2":
				q.Question = getTextContent(n)
				for child := n.FirstChild; child != nil; child = child.NextSibling {
					visitNode(child)
				}
			case "textarea":
				p := Properties{
					Type:          "textarea",
					PropertyIndex: 0,
				}
				for _, attr := range n.Attr {
					switch attr.Key {
					case "id":
						parts := strings.Split(attr.Val, "-")
						idx, err := strconv.Atoi(parts[len(parts)-1])
						if err != nil {
							fmt.Println("Error parsing question id:", err)
							idx = 0
						}
						q.QuestionIndex = idx
						q.QuestionID = idx
						q.QuestionName = attr.Val
					}
				}
				props = append(props, p)
			case "span":
				for _, attr := range n.Attr {
					if attr.Key == "class" && strings.Contains(attr.Val, "required") {
						q.Required = true
					}
				}
			default:
				for child := n.FirstChild; child != nil; child = child.NextSibling {
					visitNode(child)
				}
			}

		}
	}
	visitNode(n)
	q.Properties = props
	return q
}

func getTextContent(n *html.Node) string {
	var textContent string
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			textContent += c.Data
		}
	}
	return strings.TrimSpace(textContent)
}
