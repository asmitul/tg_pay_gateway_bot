// Package domain defines shared domain constants and types.
package domain

const (
	// RoleOwner represents the bot owner with the highest privileges.
	RoleOwner = "owner"
	// RoleAdmin represents elevated administrators below the owner.
	RoleAdmin = "admin"
	// RoleUser represents a standard user with no elevated privileges.
	RoleUser = "user"
)
