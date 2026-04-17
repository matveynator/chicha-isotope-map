package desktop

import (
	"reflect"
	"testing"
)

func TestParseAppleScriptSelectedPaths(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:  "newline paths",
			input: "/Users/matvey/track.json\n/Users/matvey/track2.gpx\n",
			expected: []string{
				"/Users/matvey/track.json",
				"/Users/matvey/track2.gpx",
			},
		},
		{
			name:  "quoted entries",
			input: "\"/Users/matvey/tQn4le.json\"\n",
			expected: []string{
				"/Users/matvey/tQn4le.json",
			},
		},
		{
			name:  "quoted entries with trailing comma",
			input: "\"/Users/matvey/tQn4le.json\",\n\"/Users/matvey/track2.gpx\",\n",
			expected: []string{
				"/Users/matvey/tQn4le.json",
				"/Users/matvey/track2.gpx",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actual := parseAppleScriptSelectedPaths(testCase.input)
			if !reflect.DeepEqual(actual, testCase.expected) {
				t.Fatalf("parseAppleScriptSelectedPaths() = %#v, expected %#v", actual, testCase.expected)
			}
		})
	}
}
