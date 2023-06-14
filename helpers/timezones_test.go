package helpers

import (
	"feedrewind/third_party/tzdata"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTzInfoIsComplete(t *testing.T) {
	for location := range tzdata.LocationByName {
		if !strings.Contains(location, "/") || strings.HasPrefix(location, "Etc/") {
			continue
		}
		_, ok := GroupIdByTimezoneId[location]
		assert.Truef(t, ok, "timezone from tzdata is not in tzdb: %s", location)
	}
}

func TestTzdbIsComplete(t *testing.T) {
	for groupId := range FriendlyNameByGroupId {
		_, ok := tzdata.LocationByName[groupId]
		assert.Truef(t, ok, "group from tzdb is not in tzdata: %s", groupId)
	}
}
