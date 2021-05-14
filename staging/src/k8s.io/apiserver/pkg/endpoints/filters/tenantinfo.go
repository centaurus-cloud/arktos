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
	"net/url"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/endpoints/handlers/responsewriters"
	"k8s.io/apiserver/pkg/endpoints/request"
)

var TenantParam = "tenant"

// WithTenantInfo set the tenant in the requestInfo for short-path requests based on the user info from the authentication result.
func WithTenantInfo(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {

		newReq, err := SetShortPathRequestTenant(req)
		if err != nil {
			responsewriters.InternalError(w, req, err)
			return
		}

		handler.ServeHTTP(w, newReq)
	})
}

// try to get it from the userInfo in the context
func GetTenantFromContext(req *http.Request) (string, error) {
	ctx := req.Context()
	requestor, exists := request.UserFrom(ctx)
	if !exists {
		return "", fmt.Errorf("The user info is missing.")
	}

	userTenant := requestor.GetTenant()
	if userTenant == metav1.TenantNone {
		return "", fmt.Errorf("The tenant in the user info of %s is empty. ", requestor.GetName())
	}

	return userTenant, nil
}

func AddTenantParamToUrl(urlString string, tenant string) string {
	u, _ := url.Parse(urlString)
	queries := u.Query()
	queries.Set(TenantParam, tenant)
	u.RawQuery = queries.Encode()

	return u.String()
}

func DelTenantParamFromUrl(urlString string) string {
	u, _ := url.Parse(urlString)
	queries := u.Query()
	queries.Del(TenantParam)
	u.RawQuery = queries.Encode()

	return u.String()
}

func GetTenantFromUrlParam(urlString string) string {
	u, _ := url.Parse(urlString)
	tenantValues, ok := u.Query()[TenantParam]
	if ok {
		return tenantValues[0]
	}

	return ""
}

// This func tries to get tenant from the url param
// if not found, try to get it from the userInfo in the context
// Note: Only non-resource request (namely the group/version handlers) will try to get tenant from url param.
func GetTenantFromQueryThenContext(req *http.Request) (string, error) {
	tenant := GetTenantFromUrlParam(req.URL.String())
	if len(tenant) > 0 {
		return tenant, nil
	}

	return GetTenantFromContext(req)
}

// SetShortPathRequestTenant sets the tenant in request info based on the user tenant.
func SetShortPathRequestTenant(req *http.Request) (*http.Request, error) {
	userTenant, err := GetTenantFromContext(req)
	if err != nil {
		return nil, err
	}

	ctx := req.Context()
	requestInfo, exists := request.RequestInfoFrom(ctx)
	if !exists {
		return nil, fmt.Errorf("The request info is missing.")
	}

	// for a reqeust from a regular user, if the tenant in the object is empty, use the tenant from user info
	// this is what we call "short-path", which allows users to use traditional Kubernets API in the multi-tenancy Arktos
	resourceTenant := requestInfo.Tenant
	if resourceTenant == metav1.TenantNone {
		requestInfo.Tenant = userTenant
	}

	// regular tenants can only access his own space
	if resourceTenant == metav1.TenantAll && userTenant != metav1.TenantSystem {
		requestInfo.Tenant = userTenant
	}

	req = req.WithContext(request.WithRequestInfo(ctx, requestInfo))

	return req, nil
}
