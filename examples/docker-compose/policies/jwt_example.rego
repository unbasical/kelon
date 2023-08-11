package applications.jwt

# Deny all by default
default allow = false

# Path: GET /api/pure/apps/:app_id
# The first app is public
allow = true {
    input.method == "GET"
    input.path == ["api", "pure", "apps", "1"]
}

# Path: GET /api/pure/*
# All other paths are only accessible if Token is valid
allow = true {
    jwt_validate(input.token, ["example"])
}
