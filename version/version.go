package version

const Tag = "v0.1.0" // Bump with release tags.

// Build can be overridden at link time with:
//
//	-ldflags "-X seedetcher.com/version.Build=$(git describe --tags --dirty --always)"
var Build string

// String returns the build override if present, otherwise the release tag.
func String() string {
	if Build != "" {
		return Build
	}
	return Tag
}
