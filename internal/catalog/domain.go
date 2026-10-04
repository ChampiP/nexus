// Package catalog is the module that owns organizations, clients and projects.
package catalog

// Organization groups clients.
type Organization struct {
	ID         int64
	UID        string
	Name       string
	ArchivedAt *int64
}

// Client belongs to at most one organization.
type Client struct {
	ID             int64
	UID            string
	Name           string
	OrganizationID *int64
	ArchivedAt     *int64
}

// Project belongs to at most one client. Project names are globally unique, ignoring case.
type Project struct {
	ID         int64
	UID        string
	Name       string
	ClientID   *int64
	ArchivedAt *int64
}
