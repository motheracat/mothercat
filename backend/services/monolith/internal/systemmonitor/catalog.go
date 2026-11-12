package systemmonitor

import (
	"net/http"
	"sort"
	"strings"
)

type endpointCatalog struct {
	mux       *http.ServeMux
	endpoints map[string]Endpoint
}

func newEndpointCatalog() *endpointCatalog {
	catalog := &endpointCatalog{
		mux:       http.NewServeMux(),
		endpoints: make(map[string]Endpoint),
	}
	register := func(pattern, description, controller string) {
		parts := strings.SplitN(pattern, " ", 2)
		endpoint := Endpoint{
			Method: parts[0], Route: parts[1], Description: description, Controller: controller,
			Pausable: !protectedEndpoint(parts[0], parts[1]),
		}
		catalog.endpoints[pattern] = endpoint
		catalog.mux.HandleFunc(pattern, func(http.ResponseWriter, *http.Request) {})
	}

	register("GET /health/live", "Check API process liveness", "health")
	register("GET /health/ready", "Check API and PostgreSQL readiness", "health")
	register("POST /api/v1/auth/bootstrap", "Create the first SuperAdmin account", "identity")
	register("POST /api/v1/auth/login", "Authenticate a staff account", "identity")
	register("GET /api/v1/auth/session", "Resolve the current staff session", "identity")
	register("POST /api/v1/auth/logout", "Revoke the current staff session", "identity")
	register("POST /api/v1/auth/password/forgot", "Request password recovery", "identity")
	register("POST /api/v1/auth/password/reset", "Complete password recovery", "identity")
	register("POST /api/v1/auth/password/change", "Change the current account password", "identity")
	register("POST /api/v1/auth/activation/complete", "Complete account activation", "identity")
	register("POST /api/v1/auth/accounts/{staffID}/suspend", "Suspend a staff account", "identity")
	register("POST /api/v1/auth/accounts/{staffID}/reactivate", "Reactivate a staff account", "identity")
	register("POST /api/v1/auth/applicant/register", "Register a public applicant", "applicant-identity")
	register("POST /api/v1/auth/applicant/verify-email", "Verify an applicant email address", "applicant-identity")
	register("POST /api/v1/auth/applicant/resend-verification", "Resend applicant email verification", "applicant-identity")
	register("POST /api/v1/auth/applicant/login", "Authenticate an applicant account", "applicant-identity")
	register("POST /internal/v1/accounts", "Provision an inactive staff account", "identity-internal")
	register("POST /internal/v1/accounts/{accountID}/activation", "Send an account activation", "identity-internal")

	register("GET /api/v1/staff", "Search and list staff profiles", "organization")
	register("POST /api/v1/staff", "Create a staff profile", "organization")
	register("GET /api/v1/staff/{staffId}", "Read a staff profile", "organization")
	register("PATCH /api/v1/staff/{staffId}", "Update a staff profile", "organization")
	register("POST /api/v1/staff/{staffId}/profile-picture", "Upload a staff profile picture", "organization")
	register("POST /api/v1/staff/{staffId}/deactivate", "Deactivate a staff profile", "organization")
	register("POST /api/v1/staff/imports", "Import a staff directory", "organization")
	register("GET /api/v1/staff/imports/{importId}", "Read a staff import result", "organization")
	register("POST /api/v1/staff/{staffID}/provision", "Provision a staff account", "organization")
	register("POST /api/v1/staff/{staffID}/provision-admin", "Provision an Admin account", "organization")
	register("GET /api/v1/departments", "List departments", "organization")
	register("POST /api/v1/departments", "Create a department", "organization")
	register("PATCH /api/v1/departments/{departmentId}", "Update a department", "organization")

	register("GET /api/v1/roles", "List access roles", "access")
	register("POST /api/v1/roles", "Create an access role", "access")
	register("PATCH /api/v1/roles/{roleID}", "Update an access role", "access")
	register("GET /api/v1/permissions", "List access permissions", "access")
	register("GET /api/v1/users/role-assignments", "List user role assignments", "access")
	register("PUT /api/v1/roles/{roleID}/permissions", "Replace role permissions", "access")
	register("GET /api/v1/users/{userID}/roles", "Read a user's roles", "access")
	register("PUT /api/v1/users/{userID}/roles", "Replace a user's roles", "access")

	register("GET /api/v1/audit-events", "Search audit events", "audit")
	register("GET /api/v1/audit-events/{eventID}", "Read an audit event", "audit")
	register("GET /api/v1/tasks", "List visible tasks", "tasks")
	register("POST /api/v1/tasks", "Create a task", "tasks")
	register("GET /api/v1/me/tasks", "List tasks for the current user", "tasks")
	register("GET /api/v1/tasks/{taskID}", "Read a task", "tasks")
	register("PATCH /api/v1/tasks/{taskID}", "Update a task", "tasks")
	register("PATCH /api/v1/tasks/{taskID}/admin-status", "Update the administrative task status", "tasks")
	register("POST /api/v1/tasks/{taskID}/assign", "Assign a task", "tasks")
	register("POST /api/v1/tasks/{taskID}/start", "Start a task", "tasks")
	register("POST /api/v1/tasks/{taskID}/submit", "Submit task work", "tasks")
	register("POST /api/v1/tasks/{taskID}/approve", "Approve task work", "tasks")
	register("POST /api/v1/tasks/{taskID}/request-changes", "Request task changes", "tasks")
	register("POST /api/v1/tasks/{taskID}/reject", "Reject task work", "tasks")
	register("POST /api/v1/tasks/{taskID}/cancel", "Cancel a task", "tasks")
	register("GET /api/v1/tasks/{taskID}/history", "Read task status history", "tasks")
	register("GET /api/v1/tasks/{taskID}/comments", "List task comments", "tasks")
	register("POST /api/v1/tasks/{taskID}/comments", "Add a task comment", "tasks")

	register("GET /api/v1/notifications", "List current-user notifications", "notifications")
	register("GET /api/v1/notifications/unread-count", "Count unread notifications", "notifications")
	register("POST /api/v1/notifications/{notificationID}/read", "Mark a notification as read", "notifications")
	register("POST /api/v1/notifications/read-all", "Mark all notifications as read", "notifications")
	register("GET /api/v1/notifications/preferences", "Read notification preferences", "notifications")
	register("PUT /api/v1/notifications/preferences", "Update notification preferences", "notifications")

	register("GET /api/v1/opportunities", "List public opportunities", "opportunities")
	register("GET /api/v1/opportunities/manage", "List opportunities for management", "opportunities")
	register("POST /api/v1/opportunities", "Create an opportunity", "opportunities")
	register("GET /api/v1/opportunities/{opportunityID}", "Read an opportunity", "opportunities")
	register("PATCH /api/v1/opportunities/{opportunityID}", "Update an opportunity", "opportunities")
	register("POST /api/v1/opportunities/{opportunityID}/publish", "Publish an opportunity", "opportunities")
	register("POST /api/v1/opportunities/{opportunityID}/unpublish", "Unpublish an opportunity", "opportunities")
	register("POST /api/v1/opportunities/{opportunityID}/close", "Close an opportunity", "opportunities")
	register("POST /api/v1/opportunities/{opportunityID}/siwes-applications", "Submit a SIWES application", "opportunities")
	register("GET /api/v1/siwes-applications", "List SIWES applications", "opportunities")
	register("GET /api/v1/siwes-applications/{applicationID}", "Read a SIWES application", "opportunities")
	register("PATCH /api/v1/siwes-applications/{applicationID}", "Update a SIWES application", "opportunities")
	register("GET /api/v1/siwes-applications/{applicationID}/document", "Download an application document", "opportunities")

	register("GET /api/v1/system-monitor/overview", "Read live process and database health", "system-monitor")
	register("GET /api/v1/system-monitor/endpoints", "Read the endpoint registry and health", "system-monitor")
	register("GET /api/v1/system-monitor/events", "Read recent process-local system events", "system-monitor")
	register("POST /api/v1/system-monitor/endpoints/pause", "Temporarily pause an application endpoint", "system-monitor")
	register("POST /api/v1/system-monitor/endpoints/resume", "Resume a paused application endpoint", "system-monitor")
	register("GET /api/v1/system-monitor/endpoints/history", "Read endpoint control history", "system-monitor")
	return catalog
}

func protectedEndpoint(method, route string) bool {
	for _, prefix := range []string{
		"/health/",
		"/api/v1/auth/",
		"/internal/",
		"/api/v1/system-monitor/",
	} {
		if strings.HasPrefix(route, prefix) {
			return true
		}
	}
	return false
}

func (c *endpointCatalog) Resolve(method, path string) (Endpoint, bool) {
	request, err := http.NewRequest(method, path, nil)
	if err != nil {
		return Endpoint{}, false
	}
	_, pattern := c.mux.Handler(request)
	endpoint, exists := c.endpoints[pattern]
	return endpoint, exists
}

func (c *endpointCatalog) Lookup(method, route string) (Endpoint, bool) {
	endpoint, exists := c.endpoints[endpointKey(Endpoint{Method: method, Route: route})]
	return endpoint, exists
}

func (c *endpointCatalog) Endpoints() []Endpoint {
	endpoints := make([]Endpoint, 0, len(c.endpoints))
	for _, endpoint := range c.endpoints {
		endpoints = append(endpoints, endpoint)
	}
	sort.Slice(endpoints, func(i, j int) bool {
		if endpoints[i].Route != endpoints[j].Route {
			return endpoints[i].Route < endpoints[j].Route
		}
		return endpoints[i].Method < endpoints[j].Method
	})
	return endpoints
}
