package routes

import (
	"feedrewind/routes/rutil"
	"feedrewind/util"
	"fmt"
	"net/http"
)

func AdminTest_DestroyUserSubscriptions(w http.ResponseWriter, r *http.Request) {
	currentUserId := rutil.CurrentUserId(r)
	conn := rutil.DBConn(r)
	_, err := conn.Exec(`delete from subscriptions where user_id = $1`, currentUserId)
	if err != nil {
		panic(err)
	}
	_, err = w.Write([]byte("OK"))
	if err != nil {
		panic(err)
	}
}

func AdminTest_ExecuteSql(w http.ResponseWriter, r *http.Request) {
	conn := rutil.DBConn(r)
	query := util.EnsureParamStr(r, "query")
	jsonQuery := fmt.Sprintf(`
		with result_rows as (%s)
		SELECT array_to_json(array_agg(row_to_json(t))) FROM result_rows t
	`, query)
	row := conn.QueryRow(jsonQuery)
	var result string
	err := row.Scan(&result)
	if err != nil {
		panic(err)
	}
	_, err = w.Write([]byte(result))
	if err != nil {
		panic(err)
	}
}
