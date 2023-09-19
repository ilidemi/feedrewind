package postmark

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"goji.io/pat"
)

const (
	transactionalDev = "transactional-dev"
)

func TestListMessageStreams(t *testing.T) {
	responseJSON := `{
		"MessageStreams": [			{
				"ID": "outbound",
				"ServerID": 123457,
				"Name": "Transactional Stream",
				"Description": "This is my stream to send transactional messages",
				"MessageStreamType": "Transactional",
				"CreatedAt": "2020-07-01T00:00:00-04:00",
				"UpdatedAt": "2020-07-05T00:00:00-04:00",
				"ArchivedAt": null,
				"ExpectedPurgeDate": null,
				"SubscriptionManagementConfiguration": {
					"UnsubscribeHandlingType": "none"
				}
			},
			{
				"ID": "inbound",
				"ServerID": 123457,
				"Name": "Inbound Stream",
				"Description": "Stream used for receiving inbound messages",
				"MessageStreamType": "Inbound",
				"CreatedAt": "2020-07-01T00:00:00-04:00",
				"UpdatedAt": null,
				"ArchivedAt": null,
				"ExpectedPurgeDate": null,
				"SubscriptionManagementConfiguration": {
					"UnsubscribeHandlingType": "none"
				}
			},
			{
				"ID": "transactional-dev",
				"ServerID": 123457,
				"Name": "My Dev Transactional Stream",
				"Description": "This is my second transactional stream",
				"MessageStreamType": "Transactional",
				"CreatedAt": "2020-07-02T00:00:00-04:00",
				"UpdatedAt": "2020-07-04T00:00:00-04:00",
				"ArchivedAt": null,
				"ExpectedPurgeDate": null,
				"SubscriptionManagementConfiguration": {
					"UnsubscribeHandlingType": "none"
				}
			}
		],
		"TotalCount": 3
	}`

	tMux.HandleFunc(pat.Get("/message-streams"), func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Query().Get("IncludeArchivedStreams") != "false" {
			t.Fatalf("MessageStreams: wrong IncludeArchivedStreams value (%s)", req.URL.Query().Get("IncludeArchivedStreams"))
		}
		if req.URL.Query().Get("MessageStreamType") != "All" {
			t.Fatalf("MessageStreams: wrong messageStreamType value (%s)", req.URL.Query().Get("MessageStreamType"))
		}
		_, _ = w.Write([]byte(responseJSON))
	})

	res, err := client.ListMessageStreams(context.Background(), "All", false)
	if err != nil {
		t.Fatalf("MessageStreams: %s", err.Error())
	}

	if len(res) != 3 {
		t.Fatalf("MessageStreams: wrong number of message streams (%d)", len(res))
	}

	// For each message stream, check the ServerID
	for _, ms := range res {
		if ms.ServerID != 123457 {
			t.Fatalf("MessageStreams: wrong ServerID (%d)", ms.ServerID)
		}
		if ms.ArchivedAt != nil {
			t.Fatalf("MessageStreams: wrong ArchivedAt (%s)", *ms.ArchivedAt)
		}
	}

	if res[0].ID != "outbound" {
		t.Fatalf("MessageStreams: wrong ID (%s)", res[0].ID)
	}
	if res[1].ID != "inbound" {
		t.Fatalf("MessageStreams: wrong ID (%s)", res[1].ID)
	}
	if res[2].ID != transactionalDev {
		t.Fatalf("MessageStreams: wrong ID (%s)", res[2].ID)
	}
}

func TestGetUnknownMessageStream(t *testing.T) {
	responseJSON := `{"ErrorCode":1226,"Message":"The message stream for the provided 'ID' was not found."}`

	tMux.HandleFunc(pat.Get("/message-streams/unknown"), func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(responseJSON))
	})

	res, err := client.GetMessageStream(context.Background(), "unknown")
	if err == nil {
		t.Fatalf("MessageStream: expected error")
	}
	if err.Error() != "The message stream for the provided 'ID' was not found." {
		t.Fatalf("MessageStream: wrong error message (%s)", err.Error())
	}

	var zero MessageStream
	if res != zero {
		t.Fatalf("MessageStream: expected empty response")
	}
}

func TestGetMessageStream(t *testing.T) {
	responseJSON := `{
		"ID": "broadcasts",
		"ServerID": 123456,
		"Name": "Broadcast Stream",
		"Description": "This is my stream to send broadcast messages",
		"MessageStreamType": "Broadcasts",
		"CreatedAt": "2020-07-01T00:00:00-04:00",
		"UpdatedAt": "2020-07-01T00:00:00-04:00",
		"ArchivedAt": null,
		"ExpectedPurgeDate": null,
		"SubscriptionManagementConfiguration": {
			"UnsubscribeHandlingType": "Postmark"
		}
	}`

	tMux.HandleFunc(pat.Get("/message-streams/broadcasts"), func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(responseJSON))
	})

	res, err := client.GetMessageStream(context.Background(), "broadcasts")
	if err != nil {
		t.Fatalf("MessageStream: %s", err.Error())
	}

	if res.ID != "broadcasts" {
		t.Fatalf("MessageStream: wrong ID (%s)", res.ID)
	}

	if res.Name != "Broadcast Stream" {
		t.Fatalf("MessageStream: wrong Name (%s)", res.Name)
	}

	if *res.Description != "This is my stream to send broadcast messages" {
		t.Fatalf("MessageStream: wrong Description (%s)", *res.Description)
	}
}

func TestEditMessageStream(t *testing.T) {
	responseJSON := `{
		"ID": "transactional-dev",
		"ServerID": 123457,
		"Name": "Updated Dev Stream",
		"Description": "Updating my dev transactional stream",
		"MessageStreamType": "Transactional",
		"CreatedAt": "2020-07-02T00:00:00-04:00",
		"UpdatedAt": "2020-07-03T00:00:00-04:00",
		"ArchivedAt": null,
		"ExpectedPurgeDate": null,
		"SubscriptionManagementConfiguration": {
			"UnsubscribeHandlingType": "none"
		}
	}`

	editReq := EditMessageStreamRequest{
		Name: "Updated Dev Stream",
		SubscriptionManagementConfiguration: MessageStreamSubscriptionManagementConfiguration{
			UnsubscribeHandlingType: "none",
		},
	}

	tMux.HandleFunc(pat.Patch("/message-streams/transactional-dev"), func(w http.ResponseWriter, req *http.Request) {
		var body EditMessageStreamRequest
		err := json.NewDecoder(req.Body).Decode(&body)
		if err != nil {
			t.Fatalf("Failed to read request body: %s", err.Error())
		}

		if body.Description != nil {
			t.Fatalf("EditMessageStream: wrong Description (%v)", body.Description)
		}
		if editReq.Name != body.Name {
			t.Fatalf("EditMessageStream: wrong Name (%s)", body.Name)
		}
		if editReq.SubscriptionManagementConfiguration.UnsubscribeHandlingType != body.SubscriptionManagementConfiguration.UnsubscribeHandlingType {
			t.Fatalf("EditMessageStream: wrong UnsubscribeHandlingType (%s)", body.SubscriptionManagementConfiguration.UnsubscribeHandlingType)
		}

		_, _ = w.Write([]byte(responseJSON))
	})

	res, err := client.EditMessageStream(context.Background(), transactionalDev, editReq)
	if err != nil {
		t.Fatalf("MessageStream: %s", err.Error())
	}

	if res.ID != transactionalDev {
		t.Fatalf("MessageStream: wrong ID (%s)", res.ID)
	}
	if res.ServerID != 123457 {
		t.Fatalf("MessageStream: wrong ServerID (%d)", res.ServerID)
	}
	if *res.Description != "Updating my dev transactional stream" {
		t.Fatalf("MessageStream: wrong Description (%s)", *res.Description)
	}
}

func TestCreateMessageStream(t *testing.T) {
	responseJSON := `{
		"ID": "transactional-dev",
		"ServerID": 123457,
		"Name": "My Dev Transactional Stream",
		"Description": "This is my second transactional stream",
		"MessageStreamType": "Transactional",
		"CreatedAt": "2020-07-02T00:00:00-04:00",
		"UpdatedAt": "2020-07-02T00:00:00-04:00",
		"ArchivedAt": "2020-07-02T00:00:00-04:00",
		"SubscriptionManagementConfiguration": {
			"UnsubscribeHandlingType": "None"
		}
	}`

	desc := "This is my second transactional stream"
	createReq := CreateMessageStreamRequest{
		ID:                transactionalDev,
		Name:              "My Dev Transactional Stream",
		Description:       &desc,
		MessageStreamType: "Transactional",
		SubscriptionManagementConfiguration: MessageStreamSubscriptionManagementConfiguration{
			UnsubscribeHandlingType: "None",
		},
	}

	tMux.HandleFunc(pat.Post("/message-streams"), func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(responseJSON))
	})

	res, err := client.CreateMessageStream(context.Background(), createReq)
	if err != nil {
		t.Fatalf("MessageStream: %s", err.Error())
	}

	if res.ID != transactionalDev {
		t.Fatalf("MessageStream: wrong ID (%s)", res.ID)
	}
	if res.ServerID != 123457 {
		t.Fatalf("MessageStream: wrong ServerID (%d)", res.ServerID)
	}
	if res.MessageStreamType != "Transactional" {
		t.Fatalf("MessageStream: wrong MessageStreamType (%s)", res.MessageStreamType)
	}
}

func TestArchiveMessageStream(t *testing.T) {
	responseJSON := `{
		"ID": "transactional-dev",
		"ServerID": 123457,
		"ExpectedPurgeDate": "2020-08-30T12:30:00.00-04:00"
	}`

	tMux.HandleFunc(pat.Post("/message-streams/transactional-dev/archive"), func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(responseJSON))
	})

	res, err := client.ArchiveMessageStream(context.Background(), transactionalDev)
	if err != nil {
		t.Fatalf("MessageStream: %s", err.Error())
	}

	if res.ID != transactionalDev {
		t.Fatalf("MessageStream: wrong ID (%s)", res.ID)
	}
	if res.ServerID != 123457 {
		t.Fatalf("MessageStream: wrong ServerID (%d)", res.ServerID)
	}
	if res.ExpectedPurgeDate != "2020-08-30T12:30:00.00-04:00" {
		t.Fatalf("MessageStream: wrong ExpectedPurgeDate (%s)", res.ExpectedPurgeDate)
	}
}

func TestUnarchiveMessageStream(t *testing.T) {
	responseJSON := `{
		"ID": "transactional-dev",
		"ServerID": 123457,
		"Name": "Updated Dev Stream",
		"Description": "Updating my dev transactional stream",
		"MessageStreamType": "Transactional",
		"CreatedAt": "2020-07-02T00:00:00-04:00",
		"UpdatedAt": "2020-07-04T00:00:00-04:00",
		"ArchivedAt": null,
		"SubscriptionManagementConfiguration": {
			"UnsubscribeHandlingType": "none"
		}
	}`

	tMux.HandleFunc(pat.Post("/message-streams/transactional-dev/unarchive"), func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(responseJSON))
	})

	res, err := client.UnarchiveMessageStream(context.Background(), transactionalDev)
	if err != nil {
		t.Fatalf("MessageStream: %s", err.Error())
	}

	if res.ID != transactionalDev {
		t.Fatalf("MessageStream: wrong ID (%s)", res.ID)
	}
	if res.ServerID != 123457 {
		t.Fatalf("MessageStream: wrong ServerID (%d)", res.ServerID)
	}
}
