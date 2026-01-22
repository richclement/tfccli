// Package config manages CLI settings stored in ~/.tfccli/settings.json.
//
// Settings support multiple named contexts, each with its own address,
// default organization, and log level. The CurrentContext field tracks
// which context is active.
package config
