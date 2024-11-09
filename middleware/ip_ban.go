package middleware

import (
	"net/http"
	"sync/atomic"
	"time"

	"feedrewind.com/db"
	"feedrewind.com/log"
	"feedrewind.com/util"
)

var bannedIps atomic.Pointer[map[string]bool]

func IpBan(next http.Handler) http.Handler {
	if bannedIps.Load() == nil {
		panic("Banned ips not set")
	}
	fn := func(w http.ResponseWriter, r *http.Request) {
		userIp := util.UserIp(r)
		if (*bannedIps.Load())[userIp] {
			GetLogger(r).Info().Msg("ip is banned")
			w.WriteHeader(http.StatusNotFound)
			return
		}
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

func MustInitBannedIps() {
	logger := &log.TaskLogger{
		TaskName: "poll_banned_ips",
	}
	err := readBannedIps(logger)
	if err != nil {
		panic(err)
	}
	go func() {
		timer := time.Tick(10 * time.Second)
		for range timer {
			err := readBannedIps(logger)
			if err != nil {
				logger.Warn().Err(err).Msgf("Couldn't refresh banned ips")
			}
		}
	}()
}

func readBannedIps(logger log.Logger) error {
	rows, err := db.RootPool.Query(`select ip from banned_ips`)
	if err != nil {
		return err
	}
	newBannedIps := map[string]bool{}
	for rows.Next() {
		var ip string
		err := rows.Scan(&ip)
		if err != nil {
			return err
		}
		newBannedIps[ip] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	prevBannedIps := bannedIps.Load()
	if prevBannedIps != nil {
		for prevIp := range *prevBannedIps {
			if !newBannedIps[prevIp] {
				logger.Info().Msgf("Removed banned ip: %s", prevIp)
			}
		}
		for newIp := range newBannedIps {
			if !(*prevBannedIps)[newIp] {
				logger.Info().Msgf("Added banned ip: %s", newIp)
			}
		}
	} else if len(newBannedIps) > 0 {
		logger.Info().Msgf("Read banned ips: %v", newBannedIps)
	}
	bannedIps.Store(&newBannedIps)

	return nil
}
