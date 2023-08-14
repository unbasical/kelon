package applications.authn

# Deny all by default
default allow := false

# Path: GET /api/authn/apps/:app_id
# The first app is public
allow {
	input.method == "GET"
	input.path == ["api", "authn", "apps", "1"]
}

allow {
	jwt_verify(input.token, ["example"])
}