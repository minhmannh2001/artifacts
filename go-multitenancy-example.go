package main

import (
	"fmt"
	"net/http"

	"github.com/fatih/color"
	"github.com/gorilla/mux"
	"github.com/streadway/simpleuuid"
	"github.com/vattle/go-multitenancy"
)

func main() {
	r := mux.NewRouter()

	tenantManager := multitenancy.NewTenantManager(nil, nil)

	r.Use(tenantManager.TenantMiddleware)

	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		tenant := multitenancy.TenantFromContext(r.Context())
		fmt.Fprintf(w, "Hello, you're accessing tenant: %s", tenant.ID)
	})

	http.ListenAndServe(":8080", r)
}

func init() {
	color.NoColor = false
}

type TenantResolver struct{}

func (tr *TenantResolver) ResolveTenant(r *http.Request) (*multitenancy.Tenant, error) {
	tenantID := r.Header.Get("X-Tenant-ID")
	if tenantID == "" {
		return nil, fmt.Errorf("no tenant ID provided")
	}

	uuid, err := simpleuuid.NewString(tenantID)
	if err != nil {
		return nil, fmt.Errorf("invalid tenant ID: %v", err)
	}

	return &multitenancy.Tenant{
		ID: uuid,
	}, nil
}
