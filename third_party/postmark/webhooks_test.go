package postmark

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"goji.io/pat"
)

const (
	outbound = "outbound"
)

func TestGetWebhooks(t *testing.T) {
	responseJSON := `{ 
		"Webhooks": [
			{
				"ID": 1234567, 
				"Url": "http://www.example.com/webhook-test-tracking",
				"MessageStream": "outbound",
				"HttpAuth":{ 
					"Username": "user",
					"Password": "pass"
				},
				"HttpHeaders":[
					{
						"Name": "name",
						"Value": "value"
					}
				],
				"Triggers": { 
					"Open":{ 
						"Enabled": true,
						"PostFirstOpenOnly": false
					},
					"Click":{ 
						"Enabled": true
					},
					"Delivery":{ 
						"Enabled": true
					},
					"Bounce":{ 
						"Enabled": false,
						"IncludeContent": false
					},
					"SpamComplaint":{ 
						"Enabled": false,
						"IncludeContent": false
					},
					"SubscriptionChange": {
						"Enabled": true
					}
				}
			},
			{
				"ID": 1234568, 
				"Url": "http://www.example.com/webhook-test-bounce",
				"MessageStream": "outbound",
				"HttpAuth":{ 
					"Username": "user",
					"Password": "pass"
				},
				"HttpHeaders":[
					{
						"Name": "name",
						"Value": "value"
					}
				],
				"Triggers": { 
					"Open":{ 
						"Enabled":false,
						"PostFirstOpenOnly":false
					},
					"Click":{ 
						"Enabled": false
					},
					"Delivery":{ 
						"Enabled": false
					},
					"Bounce":{ 
						"Enabled" :true,
						"IncludeContent": false
					},
					"SpamComplaint":{ 
						"Enabled": false,
						"IncludeContent": false
					},
					"SubscriptionChange": {
						"Enabled": false
					}
				}
			}
		]
	}`

	tMux.HandleFunc(pat.Get("/webhooks"), func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(responseJSON))
	})

	res, err := client.ListWebhooks(context.Background(), "")
	if err != nil {
		t.Fatalf("Webhook: %s", err.Error())
	}

	if len(res) != 2 {
		t.Fatalf("Webhook: wrong number of webhooks listed!")
	}
	if res[0].ID != 1234567 || res[1].ID != 1234568 {
		t.Fatalf("Webhook: wrong ID!")
	}
}

func TestGetWebhook(t *testing.T) {
	responseJSON := `{
		"ID": 1234567, 
		"Url": "http://www.example.com/webhook-test-tracking",
		"MessageStream": "outbound",
		"HttpAuth":{ 
			"Username": "user",
			"Password": "pass"
		},
		"HttpHeaders":[
			{
				"Name": "name",
				"Value": "value"
			}
		],
		"Triggers": { 
			"Open":{ 
				"Enabled": true,
				"PostFirstOpenOnly": false
			},
			"Click":{ 
				"Enabled": true
			},
			"Delivery":{ 
				"Enabled": true
			},
			"Bounce":{ 
				"Enabled": false,
				"IncludeContent": false
			},
			"SpamComplaint":{ 
				"Enabled": false,
				"IncludeContent": false
			},
			"SubscriptionChange": {
				"Enabled": true
			}
		}
	}`

	tMux.HandleFunc(pat.Get("/webhooks/:webhookID"), func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(responseJSON))
	})

	res, err := client.GetWebhook(context.Background(), 1234567)
	if err != nil {
		t.Fatalf("Webhook: %s", err.Error())
	}

	if res.ID != 1234567 {
		t.Fatalf("Webhook: wrong ID!")
	}
	if res.MessageStream != outbound {
		t.Fatalf("Webhook: wrong message stream!")
	}
	if res.HTTPHeaders[0].Name != "name" {
		t.Fatalf("Webhook: wrong HTTpHeaders!")
	}
	if !res.Triggers.SubscriptionChange.Enabled {
		t.Fatalf("Webhook: wrong Subscription Change trigger state!")
	}
}

func TestCreateWebhook(t *testing.T) {
	webhook := Webhook{
		URL:           "http://www.example.com/webhook-test-tracking",
		MessageStream: outbound,
		HTTPAuth: &WebhookHTTPAuth{
			Username: "user",
			Password: "pass",
		},
		HTTPHeaders: []Header{
			{
				Name:  "name",
				Value: "value",
			},
		},
		Triggers: WebhookTrigger{
			Open: WebhookTriggerOpen{
				WebhookTriggerEnabled: WebhookTriggerEnabled{
					Enabled: true,
				},
				PostFirstOpenOnly: true,
			},
			Click: WebhookTriggerEnabled{
				Enabled: true,
			},
		},
	}

	tMux.HandleFunc(pat.Post("/webhooks"), func(w http.ResponseWriter, req *http.Request) {
		decoder := json.NewDecoder(req.Body)

		var res Webhook
		err := decoder.Decode(&res)
		req.Body.Close()

		if err != nil {
			t.Fatalf("Webhook: %s", err.Error())
		}

		if res.MessageStream != outbound {
			t.Fatalf("Webhook: wrong message stream!")
		}
		if !res.Triggers.Open.Enabled {
			t.Fatalf("Webhook: wrong Open trigger state!")
		}

		res.ID = 12345

		resBytes, err := json.Marshal(res)
		if err != nil {
			t.Fatalf("Webhook: %s", err.Error())
		}

		_, _ = w.Write(resBytes)
	})

	res, err := client.CreateWebhook(context.Background(), webhook)
	if err != nil {
		t.Fatalf("Webhook: %s", err.Error())
	}

	if res.ID != 12345 {
		t.Fatalf("Webhook: wrong ID!")
	}
	if res.MessageStream != outbound {
		t.Fatalf("Webhook: wrong message stream!")
	}
	if !res.Triggers.Open.Enabled {
		t.Fatalf("Webhook: wrong Open trigger state!")
	}
}

func TestEditWebhook(t *testing.T) {
	webhook := Webhook{
		URL:           "http://www.example.com/webhook-test-tracking",
		MessageStream: outbound,
		HTTPAuth: &WebhookHTTPAuth{
			Username: "user",
			Password: "pass",
		},
		HTTPHeaders: []Header{
			{
				Name:  "name",
				Value: "value",
			},
		},
		Triggers: WebhookTrigger{
			Open: WebhookTriggerOpen{
				WebhookTriggerEnabled: WebhookTriggerEnabled{
					Enabled: true,
				},
				PostFirstOpenOnly: true,
			},
			Click: WebhookTriggerEnabled{
				Enabled: true,
			},
		},
	}

	tMux.HandleFunc(pat.Put("/webhooks/:webhookID"), func(w http.ResponseWriter, req *http.Request) {
		decoder := json.NewDecoder(req.Body)

		var res Webhook
		err := decoder.Decode(&res)
		req.Body.Close()

		if err != nil {
			t.Fatalf("Webhook: %s", err.Error())
		}

		if res.MessageStream != outbound {
			t.Fatalf("Webhook: wrong message stream!")
		}
		if !res.Triggers.Open.Enabled {
			t.Fatalf("Webhook: wrong Open trigger state!")
		}

		res.ID = 12345

		resBytes, err := json.Marshal(res)
		if err != nil {
			t.Fatalf("Webhook: %s", err.Error())
		}

		_, _ = w.Write(resBytes)
	})

	res, err := client.EditWebhook(context.Background(), 12345, webhook)
	if err != nil {
		t.Fatalf("Webhook: %s", err.Error())
	}

	if res.ID != 12345 {
		t.Fatalf("Webhook: wrong ID!")
	}
	if res.MessageStream != outbound {
		t.Fatalf("Webhook: wrong message stream!")
	}
	if !res.Triggers.Open.Enabled {
		t.Fatalf("Webhook: wrong Open trigger state!")
	}
}

func TestDeleteWebhook(t *testing.T) {
	responseJSON := `{
	  "ErrorCode": 0,
	  "Message": "Webhook 1234 removed."
	}`

	tMux.HandleFunc(pat.Delete("/webhooks/:webhookID"), func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(responseJSON))
	})

	// Success
	err := client.DeleteWebhook(context.Background(), 1234)
	if err != nil {
		t.Fatalf("DeleteWebhook: %s", err.Error())
	}

	// Failure
	responseJSON = `{
	  "ErrorCode": 402,
	  "Message": "Invalid JSON"
	}`

	err = client.DeleteWebhook(context.Background(), 1234)
	if err == nil {
		t.Fatalf("DeleteWebhook: should have failed")
	}
}
