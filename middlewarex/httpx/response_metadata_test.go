package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestEncodeJSONResponseMetadata(t *testing.T) {
	for _, status := range []int{200, 201, 204, 409} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			body := map[string]string{"result": "ok"}
			resp := Response[map[string]string]{Status: status, Headers: http.Header{
				"X-Values": {"one", "two"}, "Set-Cookie": {"extra=value; Path=/"},
			}, Cookies: []*http.Cookie{
				{Name: "__Host-gymsid", Value: "signed", Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode},
				{Name: "gymsid", Path: "/", MaxAge: -1, HttpOnly: true}, nil,
			}}
			if status != 204 {
				resp.Body = &body
			}
			w := httptest.NewRecorder()
			w.Header().Add("X-Values", "existing")
			if err := EncodeJSON(w, httptest.NewRequest("GET", "/", nil), resp); err != nil {
				t.Fatal(err)
			}
			result := w.Result()
			defer result.Body.Close()
			if result.StatusCode != status {
				t.Fatalf("status=%d", result.StatusCode)
			}
			if !reflect.DeepEqual(result.Header.Values("X-Values"), []string{"existing", "one", "two"}) {
				t.Fatalf("headers=%v", result.Header)
			}
			if len(result.Header.Values("Set-Cookie")) != 3 {
				t.Fatalf("Set-Cookie=%v", result.Header.Values("Set-Cookie"))
			}
			cookies := result.Cookies()
			if len(cookies) != 3 || cookies[1].Name != "__Host-gymsid" || !cookies[1].Secure || !cookies[1].HttpOnly || cookies[1].Path != "/" || cookies[1].SameSite != http.SameSiteLaxMode || cookies[2].MaxAge != -1 {
				t.Fatalf("cookies=%#v", cookies)
			}
			if status == 204 {
				if w.Body.Len() != 0 {
					t.Fatal("unexpected body")
				}
			} else {
				var got map[string]string
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(got, body) {
					t.Fatalf("body=%v", got)
				}
				if result.Header.Get("Content-Type") != "application/json; charset=utf-8" {
					t.Fatalf("content type=%q", result.Header.Get("Content-Type"))
				}
			}
		})
	}
}

func TestEncodeJSONPreservesExplicitContentType(t *testing.T) {
	resp := OK(map[string]string{"error": "conflict"})
	resp.Headers = http.Header{"Content-Type": {"application/problem+json"}}
	w := httptest.NewRecorder()
	if err := EncodeJSON(w, httptest.NewRequest("GET", "/", nil), resp); err != nil {
		t.Fatal(err)
	}
	if got := w.Result().Header.Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("Content-Type=%q", got)
	}
}
