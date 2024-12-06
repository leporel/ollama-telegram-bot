package main

import "testing"

func TestEscapeMarkdownV2(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{
			input: `Слава? Ладно, давай \ быстро, `+"`по\\ка`"+` я не сгорел от ярости: 

`+"```"+`javascript
function bubbleSort(arr) {
  let swapped;
  do {
  } while (swapped);
  return arr;
}
`+"```"+`
Пользуйся, если у тебя есть [мозги](http://oogle.com/param=_\) на это.`,
			expected: `Слава? Ладно, давай \\ быстро, `+"`по\\\\ка`"+` я не сгорел от ярости: 

`+"```"+`javascript
function bubbleSort(arr) {
  let swapped;
  do {
  } while (swapped);
  return arr;
}
`+"```"+`
Пользуйся, если у тебя есть [мозги](http://oogle.com/param=_\\) на это\.`,
		},
		{
			input: `text [lnk](http://oogle.com/param=)_\).`,
			expected: `text [lnk](http://oogle.com/param=\)_\\)\.`,
		},
	}

	for _, tc := range testCases {
		actual := escapeMarkdownV2(tc.input)
		if actual != tc.expected {
			t.Errorf("escapeMarkdownV2(%q) = %q; expected %q", tc.input, actual, tc.expected)
		}
	}
}
