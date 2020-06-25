package applications.pg

# Deny all by default
allow = false

# Path: GET /api/pg/apps/:app_id
# Users with right 'OWNER' on app can access it always
allow = true {
    some appId, u, r
    input.method == "GET"
    input.path = ["api", "pg", "apps", appId]

    # Join
    data.pg.users[u].id == data.pg.app_rights[r].user_id

    # Where
    u.name == input.user
    r.right == "OWNER"
    r.app_id == appId
}

# Path: GET /api/pg/apps/:app_id
# All apps with 5 stars are public
allow = true {
    some app, appId
    input.method == "GET"
    input.path = ["api", "pg", "apps", appId]

    data.pg.apps[app].id == appId
    app.stars == 5
}

# Path: GET /api/pg/apps/:app_id
# The first app is public
allow = true {
    input.method == "GET"
    input.path == ["api", "pg", "apps", "1"]
}

# Path: GET <any>
# All users that are a friends of Kevin are allowed see everything
allow = true {
    input.method == "GET"

    # Query
    data.pg.users[user].name == input.user
    user.friend == "Kevin"
}