package util

import (
	"strings"
	"testing"

	"feedrewind.com/third_party/tzdata"

	"github.com/stretchr/testify/require"
)

func TestTzInfoIsComplete(t *testing.T) {
	for location := range tzdata.LocationByName {
		if !strings.Contains(location, "/") || strings.HasPrefix(location, "Etc/") {
			continue
		}
		_, ok := GroupIdByTimezoneId[location]
		require.Truef(t, ok, "timezone from tzdata is not in tzdb: %s", location)
	}
}

func TestTzdbIsComplete(t *testing.T) {
	for _, friendlyTimezone := range FriendlyTimezones {
		_, ok := tzdata.LocationByName[friendlyTimezone.GroupId]
		require.Truef(t, ok, "group from tzdb is not in tzdata: %s", friendlyTimezone.GroupId)
	}
}
