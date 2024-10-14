package bloomreachQaAssignment

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const timeout = 10 * time.Second

func Test_Survey(t *testing.T) {
	tests := []struct {
		name    string
		answers map[string][]string
		err     bool
	}{
		{
			"Success all fields",
			map[string][]string{
				"question-0": {"Blue"},
				"question-1": {"Pop", "Rock", "Classical", "Jazz", "Punk", "Techno"},
				"question-2": {"1"},
				"question-3": {"Lorem ipsum dolor sit amet"},
			},
			false,
		},
		{
			"Success req fields only",
			map[string][]string{
				"question-0": {"Blue"},
				"question-1": {"Pop", "Rock", "Classical", "Jazz", "Punk", "Techno"},
				"question-2": {"1"},
				"question-3": {},
			},
			false,
		},
		{
			"Error radio invalid fields",
			map[string][]string{
				"question-0": {"Something"},
				"question-1": {"Pop", "Rock", "Classical", "Jazz", "Punk", "Techno"},
				"question-2": {"1"},
				"question-3": {"Lorem ipsum dolor sit amet"},
			},
			true,
		},
		{
			"Error rating out of index",
			map[string][]string{
				"question-0": {"Something"},
				"question-1": {"Pop", "Rock", "Classical", "Jazz", "Punk", "Techno"},
				"question-2": {"9"},
				"question-3": {"Lorem ipsum dolor sit amet"},
			},
			true,
		},
		{
			"Error non-existent question",
			map[string][]string{
				"question-0": {"Something"},
				"question-1": {"Pop", "Rock", "Classical", "Jazz", "Punk", "Techno"},
				"question-2": {"1"},
				"question-5": {"Lorem ipsum dolor sit amet"},
			},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			export, err := postCustomerExport()
			require.NoError(t, err)
			require.NotEmpty(t, export.Properties.SurveyLink)
			currentEvents := export.Events
			doc, cookies, err := getSurveyForm(export.Properties.SurveyLink)
			require.NoError(t, err)
			s, err := extractSurvey(doc)
			require.NoError(t, err)
			formData := url.Values{}
			for question, answer := range tt.answers {
				for _, a := range answer {
					formData.Add(question, a)
				}
			}
			formData.Add("csrf_token", s.CsrfToken)
			status, err := submitSurvey(s.Url, formData, cookies)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, status)
			if tt.err {
				// I don't know exactly how the error cases work. This is not an ideal solution
				require.Never(t, func() bool {
					actualExport, err := postCustomerExport()
					require.NoError(t, err)
					actualEvents := actualExport.Events
					return len(currentEvents) != len(actualEvents)
				}, timeout, 1*time.Second, "New events occurred within 30 seconds when no new events were expected.")
				return
			}
			var actualEvents []Event
			require.Eventually(t, func() bool {
				actualExport, err := postCustomerExport()
				require.NoError(t, err)
				actualEvents = actualExport.Events
				return len(currentEvents)+len(tt.answers) == len(actualExport.Events)
			}, timeout, 1*time.Second, "Process did not finish within the expected time")
			newEvents := actualEvents[len(currentEvents):]
			compareEvent(t, newEvents, tt.answers, s)
		})
	}
}

func compareEvent(t *testing.T, actualEvent []Event, testData map[string][]string, s Survey) {
	r := require.New(t)
	for _, event := range actualEvent {
		checkEventFields(t, event)
		r.Equal(event.Properties.SurveyName, s.SurveyName)
		qName := fmt.Sprintf("question-%d", event.Properties.QuestionId)
		surveyProperties := s.Questions[qName]
		testValues := testData[qName]
		answer := event.Properties.Answer
		if str, ok := answer.(string); ok {
			if len(testValues) == 0 {
				r.Equal(str, "None")
			} else {
				r.Equal(testValues[0], str)
			}
		} else if str, ok := answer.([]string); ok {
			r.Subset(str, testValues)
		}

		r.Equal(event.Properties.Question, surveyProperties.Question)
		r.Equal(event.Properties.QuestionIndex, surveyProperties.QuestionIndex)
		r.Equal(event.Properties.QuestionId, surveyProperties.QuestionID)
	}
}

func checkEventFields(t *testing.T, e Event) {
	r := require.New(t)
	r.Equal(e.Type, "survey")
	r.NotEmpty(e.Timestamp)
	r.GreaterOrEqual(e.Properties.QuestionId, 0)
	r.GreaterOrEqual(e.Properties.QuestionIndex, 0)
	r.NotEmpty(e.Properties.Question)
	r.NotEmpty(e.Properties.Answer)
	r.NotEmpty(e.Properties.SurveyName)
	r.NotEmpty(e.Properties.SurveyId)
}
