package applications.pure

# Deny all by default
default allow := false

# Path: GET /api/pure/apps/:app_id
# The first app is public
allow if {
	input.method == "GET"
	input.path == ["api", "pure", "apps", "1"]
}

# Path: GET /api/pure/*
# All other paths are only accessible by Torben
allow if {
	input.user == "Torben"
}


# Path: GET /api/pure/header
# Foo header must be set to bar
allow if {
	input.method == "GET"
	input.path == ["api", "pure", "header"]
	input.Foo == "bar"
}
