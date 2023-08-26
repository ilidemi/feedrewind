package crawler

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSequenceSuffixLength(t *testing.T) {
	type test struct {
		description    string
		buckets        [][]string
		sequence       []string
		expectedLength int
	}

	tests := []test{
		{
			description: "not a suffix",
			buckets: [][]string{
				{"http://a"},
			},
			sequence:       []string{"http://b"},
			expectedLength: 0,
		},
		{
			description: "full match",
			buckets: [][]string{
				{"http://a"},
			},
			sequence:       []string{"http://a"},
			expectedLength: 1,
		},
		{
			description: "full long match",
			buckets: [][]string{
				{"http://a"}, {"http://c", "http://b"},
			},
			sequence:       []string{"http://a", "http://b", "http://c"},
			expectedLength: 3,
		},
		{
			description: "full match and beyond",
			buckets: [][]string{
				{"http://a"},
			},
			sequence:       []string{"http://a", "http://b"},
			expectedLength: 1,
		},
		{
			description: "full long match and beyond",
			buckets: [][]string{
				{"http://a"}, {"http://c", "http://b"},
			},
			sequence:       []string{"http://a", "http://b", "http://c", "http://d"},
			expectedLength: 3,
		},
		{
			description: "suffix on bucket boundary",
			buckets: [][]string{
				{"http://a"}, {"http://c", "http://b"},
			},
			sequence:       []string{"http://b", "http://c"},
			expectedLength: 2,
		},
		{
			description: "suffix is a part of a bucket, in order",
			buckets: [][]string{
				{"http://a"}, {"http://b", "http://c"},
			},
			sequence:       []string{"http://c"},
			expectedLength: 1,
		},
		{
			description: "suffix is a part of a bucket, out of order",
			buckets: [][]string{
				{"http://a"}, {"http://c", "http://b"},
			},
			sequence:       []string{"http://c"},
			expectedLength: 1,
		},
		{
			description: "suffix starts mid-bucket",
			buckets: [][]string{
				{"http://a"}, {"http://c", "http://b"}, {"http://d"},
			},
			sequence:       []string{"http://c", "http://d"},
			expectedLength: 2,
		},
		{
			description: "subsequence but not a suffix",
			buckets: [][]string{
				{"http://a"}, {"http://c", "http://b"}, {"http://d"},
			},
			sequence:       []string{"http://c"},
			expectedLength: 0,
		},
		{
			description: "full bucket subsequence but not a suffix",
			buckets: [][]string{
				{"http://a"}, {"http://c", "http://b"}, {"http://d"},
			},
			sequence:       []string{"http://b", "http://c"},
			expectedLength: 0,
		},
		{
			description: "suffix on bucket boundary and beyond",
			buckets: [][]string{
				{"http://a"}, {"http://c", "http://b"},
			},
			sequence:       []string{"http://b", "http://c", "http://d"},
			expectedLength: 2,
		},
		{
			description: "suffix starts mid-bucket and beyond",
			buckets: [][]string{
				{"http://a"}, {"http://c", "http://b"}, {"http://d"},
			},
			sequence:       []string{"http://c", "http://d", "http://e"},
			expectedLength: 2,
		},
		{
			description: "suffix with many buckets and beyond",
			buckets: [][]string{
				{"http://a"}, {"http://b"}, {"http://c"}, {"http://d"},
			},
			sequence:       []string{"http://b", "http://c", "http://d", "http://e"},
			expectedLength: 3,
		},
	}

	logger := &DummyLogger{}
	curiEqCfg := &CanonicalEqualityConfig{
		SameHosts:         nil,
		ExpectTumblrPaths: false,
	}

	for _, tc := range tests {
		var linkBuckets [][]MaybeTitledLink
		length := 0
		for _, bucket := range tc.buckets {
			var linkBucket []MaybeTitledLink
			for _, url := range bucket {
				link, ok := ToCanonicalLink(url, logger, nil)
				require.True(t, ok, tc.description)
				linkBucket = append(linkBucket, MaybeTitledLink{
					Link:       *link,
					MaybeTitle: nil,
				})
			}
			linkBuckets = append(linkBuckets, linkBucket)
			length += len(linkBucket)
		}
		feedEntryLinks := FeedEntryLinks{
			LinkBuckets:    linkBuckets,
			Length:         length,
			IsOrderCertain: true,
		}

		var seqCuris []CanonicalUri
		for _, seqUrl := range tc.sequence {
			seqLink, ok := ToCanonicalLink(seqUrl, logger, nil)
			require.True(t, ok, tc.description)
			seqCuris = append(seqCuris, seqLink.Curi)
		}

		var suffixLength int
		require.NotPanics(t, func() {
			suffixLength = feedEntryLinks.sequenceSuffixLength(seqCuris, curiEqCfg)
		}, tc.description)
		require.Equal(t, tc.expectedLength, suffixLength, tc.description)
	}
}
