package applications

# Path: GET /apps/:app_id
# Users with right 'ADMIN' can access all apps
allow = true {
    some app
    input.method == "GET"
    input.path = ["api", "v1", "apps", appId]

    [app.id, "ADMIN"] = appRights[_]
    data.pg.apps[app].stars >= 4.3
    app.id = appId
}

# Path: GET /apps/:app_id
# Users with right 'ENGINEER' can access apps with more than 4 stars
allow = true {
    some app
    input.method == "GET"
    input.path = ["api", "v1", "apps", appId]

    data.pg.apps[app].stars >= 4
    app.id = appId
}

# Path: GET /apps/:app_id
# Everybody is allowed to see the first app
allow = true {
    some id
    input.method == "GET"
    input.path = ["api", "v1", "apps", id]
    id == 1
}

# Path: GET <any>
# All users that are a friends of Kevin are allow see all apps
allow = true {
    input.method == "GET"
    input.path = ["api", "v1", "apps"]

    # Query
    data.pg.users[user].name == input.user
    user.friend == "Kevin"
}

appRights[[app, right]] {
    data.pg.users[u].name == input.user
    right := data.pg.app_rights[r].right
    app := r.app_id
    u.id == r.user_id
}