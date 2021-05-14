/*
Copyright 2020 Authors of Arktos.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package filters

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/request"
)

func getTenantedUserInfo(tenant string) user.Info {
	return &user.DefaultInfo{Name: "fake-user", Tenant: tenant}
}

type tenantInfoTestCase struct {
	Name             string
	Url              string
	UserInfo         user.Info
	ExepctedRespCode int
	ExpectedTenant   string
}

func run(t *testing.T, testCase tenantInfoTestCase) {
	// WithTenantInfo is the only handler in the handler chain
	handler := WithTenantInfo(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			return
		}))

	req, _ := http.NewRequest("GET", testCase.Url, nil)
	req.RemoteAddr = "127.0.0.1"
	req = withTestContext(req, testCase.UserInfo, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != testCase.ExepctedRespCode {
		t.Errorf("Test Case %q: expected response code %v, but got %v", testCase.Name, w.Code, testCase.ExepctedRespCode)
	}

	ctx := req.Context()
	requestInfo, _ := request.RequestInfoFrom(ctx)
	if testCase.ExpectedTenant != requestInfo.Tenant {
		t.Errorf("Test Case %q: expected tenant %q, but got %q", testCase.Name, testCase.ExpectedTenant, requestInfo.Tenant)
	}
}

func TestTenantInfoRequest(t *testing.T) {
	testCases := []tenantInfoTestCase{
		{
			Name:             "empty user info triggers Internal error",
			Url:              "/api/v1/namespaces/default/pods",
			UserInfo:         nil,
			ExepctedRespCode: 500,
			ExpectedTenant:   "",
		},
		{
			Name:             "empty tenant in user info triggers Internal error",
			Url:              "/api/v1/namespaces/default/pods",
			UserInfo:         getTenantedUserInfo(metav1.TenantNone),
			ExepctedRespCode: 500,
			ExpectedTenant:   "",
		},
		{
			Name:             "system tenant user does not change tenant in request info",
			Url:              "/api/v1/tenants/aaa/namespaces/default/pods",
			UserInfo:         getTenantedUserInfo(metav1.TenantSystem),
			ExepctedRespCode: 200,
			ExpectedTenant:   "aaa",
		},
		{
			Name:             "system tenant change empty tenant in request info",
			Url:              "/api/v1/namespaces/default/pods",
			UserInfo:         getTenantedUserInfo(metav1.TenantSystem),
			ExepctedRespCode: 200,
			ExpectedTenant:   metav1.TenantSystem,
		},
		{
			Name:             "short path: for regular tenant user, empty tenant in request info is set to the user tenant",
			Url:              "/api/v1/namespaces/default/pods",
			UserInfo:         getTenantedUserInfo("regular-user"),
			ExepctedRespCode: 200,
			ExpectedTenant:   "regular-user",
		},
		{
			Name:             "for a regular tenant user, tenant in request info is NOT changed if set - 1",
			Url:              "/api/v1/tenants/regular-user/namespaces/default/pods",
			UserInfo:         getTenantedUserInfo("regular-user"),
			ExepctedRespCode: 200,
			ExpectedTenant:   "regular-user",
		},
		{
			Name:             "for a regular tenant user, tenant in request info is NOT changed if set - 2",
			Url:              "/api/v1/tenants/another-user/namespaces/default/pods",
			UserInfo:         getTenantedUserInfo("regular-user"),
			ExepctedRespCode: 200,
			ExpectedTenant:   "another-user",
		},
		{
			Name:             "Regular User: metav1.TenantAll will be transformed to regular-user",
			Url:              fmt.Sprintf("/api/v1/tenants/%s/namespaces", metav1.TenantAll),
			UserInfo:         getTenantedUserInfo("regular-user"),
			ExepctedRespCode: 200,
			ExpectedTenant:   "regular-user",
		},
		{
			Name:             "System User: metav1.TenantAll will be transformed to metav1.TenantAll",
			Url:              fmt.Sprintf("/api/v1/tenants/%s/namespaces", metav1.TenantAll),
			UserInfo:         getTenantedUserInfo(metav1.TenantSystem),
			ExepctedRespCode: 200,
			ExpectedTenant:   metav1.TenantAll,
		},
	}

	for _, testCase := range testCases {
		run(t, testCase)
	}
}
