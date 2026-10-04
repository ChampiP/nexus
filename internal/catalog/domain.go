// Package catalog is the module that owns organizations, clients and projects.
package catalog

import "errors"

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

var (
	// ErrEmptyName indicates that a name is blank after trimming.
	ErrEmptyName = errors.New("name cannot be empty")
	// ErrDuplicateName indicates that another item in the same scope has that name, ignoring case.
	ErrDuplicateName = errors.New("name already exists")
	// ErrNotFound indicates that an organization, client or project does not exist.
	ErrNotFound = errors.New("not found")
	// ErrMergeSelf indicates an attempt to merge a project into itself.
	ErrMergeSelf = errors.New("cannot merge a project into itself")
)

// TreeOrganization is an organization with its clients; a nil Organization groups clients (and
// projects) that have no organization.
type TreeOrganization struct {
	Organization *Organization
	Clients      []TreeClient
}

// TreeClient is a client with its projects; a nil Client groups projects that have no client.
type TreeClient struct {
	Client   *Client
	Projects []Project
}
