package rutil

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPostSlug(t *testing.T) {
	type Test struct {
		Description  string
		PostTitle    string
		ExpectedSlug string
	}
	tests := []Test{
		{
			Description:  "one word",
			PostTitle:    "hello",
			ExpectedSlug: "hello",
		},
		{
			Description:  "downcase",
			PostTitle:    "Hello",
			ExpectedSlug: "hello",
		},
		{
			Description:  "two words",
			PostTitle:    "hello world",
			ExpectedSlug: "hello-world",
		},
		{
			Description:  "exclude special characters",
			PostTitle:    "hello0123456789!@#$%^&*()-=+",
			ExpectedSlug: "hello0123456789",
		},
		{
			Description:  "handle unicode",
			PostTitle:    "ด้้้้้็็็็็้้้้้็็็็็้้้้้้้้็็็็็้้้้้็็็็็้้้้้้้้็็็็็้้้้้็็็็็้้้้้้้้็็็็็้้้้้็็็็ ด้้้้้็็็็็้้้้้็็็็็้้้้้้้้็็็็็้้้้้็็็็็้้้้้้้้็็็็็้้้้้็็็็็้้้้้้้้็็็็็้้้้้็็็็ ด้้้้้็็็็็้้้้้็็็็็้้้้้้้้็็็็็้้้้้็็็็็้้้้้้้้็็็็็้้้้้็็็็็้้้้้้้้็็็็็้้้้้็็็็",
			ExpectedSlug: "",
		},
		{
			Description:  "long word",
			PostTitle:    strings.Repeat("a", 200),
			ExpectedSlug: strings.Repeat("a", 100),
		},
		{
			Description:  "limit to 10 words",
			PostTitle:    "a b c d e f g h i j k",
			ExpectedSlug: "a-b-c-d-e-f-g-h-i-j",
		},
		{
			Description:  "limit to fewer long words",
			PostTitle:    "aaaaaaaaaa bbbbbbbbbb cccccccccc dddddddddd eeeeeeeeee ffffffffff gggggggggg hhhhhhhhhh iiiiiiiiii jjjjjjjjjj",
			ExpectedSlug: "aaaaaaaaaa-bbbbbbbbbb-cccccccccc-dddddddddd-eeeeeeeeee-ffffffffff-gggggggggg-hhhhhhhhhh-iiiiiiiiii",
		},
	}

	for _, tc := range tests {
		slug := postSlug(tc.PostTitle)
		require.Equal(t, tc.ExpectedSlug, slug, tc.Description)
	}
}
