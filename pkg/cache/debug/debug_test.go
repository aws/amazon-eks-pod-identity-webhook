package debug

import (
	"encoding/json"
	"github.com/aws/amazon-eks-pod-identity-webhook/pkg/cache"
	"io"
	"io/ioutil"
	corev1 "k8s.io/api/core/v1"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// generateServiceAccount generates n service accounts with arbitrary contents
func generateServiceAccounts(n int) []*corev1.ServiceAccount {
	if n <= 0 {
		return []*corev1.ServiceAccount{}
	}
	accounts := make([]*corev1.ServiceAccount, n)
	for i := 0; i < n; i++ {
		testServiceAccount := &corev1.ServiceAccount{}
		testServiceAccount.Name = "test-sa-" + strconv.Itoa(i)
		testServiceAccount.Namespace = "default"
		testServiceAccount.Annotations = map[string]string{
			"eks.amazonaws.com/role-arn": "arn:aws:iam::111122223333:role/s3-reader-" + strconv.Itoa(i),
			"eks.amazonaws.com/audience": "sts.amazonaws.com",
		}
		accounts[i] = testServiceAccount
	}
	return accounts
}

func TestLister(t *testing.T) {
	fakeSAList := generateServiceAccounts(50000)
	emptySAList := []*corev1.ServiceAccount{}
	debugger := Dumper{
		Cache: cache.NewFakeServiceAccountCache(fakeSAList...),
	}
	ts := httptest.NewServer(
		http.HandlerFunc(debugger.Handle),
	)
	defer ts.Close()

	cases := []struct {
		caseName         string
		Cache            cache.ServiceAccountCache
		inputContentType string
		expectedLength   int
	}{
		{
			"content-type xml",
			cache.NewFakeServiceAccountCache(fakeSAList...),
			"application/xml",
			50000,
		},
		{
			"content-type json",
			cache.NewFakeServiceAccountCache(fakeSAList...),
			"application/json",
			50000,
		},
		{
			"empty cache",
			cache.NewFakeServiceAccountCache(emptySAList...),
			"application/json",
			0,
		},
	}

	for _, c := range cases {
		t.Run(c.caseName, func(t *testing.T) {
			debugger.Cache = c.Cache
			var buf io.Reader
			resp, err := http.Post(ts.URL, c.inputContentType, buf)
			if err != nil {
				t.Errorf("Failed to make request: %v", err)
				return
			}
			responseBytes, err := ioutil.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err != nil {
				t.Errorf("Failed to read response: %v", err)
				return
			}
			m := map[string]cache.CacheResponse{}
			err = json.Unmarshal(responseBytes, &m)
			if err != nil {
				t.Errorf("Failed to unmarshal: %v", err)
				return
			}
			t.Log(len(m))
			if len(m) != c.expectedLength {
				t.Errorf("Failed to receive cache contents")
			}

		})
	}
}
