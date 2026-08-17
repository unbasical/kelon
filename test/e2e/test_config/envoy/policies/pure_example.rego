package applications.pure

# Deny all by default
default allow := false

# Path: GET /api/pure/apps/:app_id
# The first app is public
allow if {
	input.method == "GET"
	input.path == ["api", "pure", "apps", "1"]
}
