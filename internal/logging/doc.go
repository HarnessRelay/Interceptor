// Package logging centralizes structured logging setup and shared field names.
//
// Log call sites should pass only safe, intentional fields. Do not log command
// environments, tokens, raw terminal input, or other secret-bearing values by
// default.
package logging
