package applications.mongo

# Deny all by default
allow = false

# Path: GET /api/mongo/apps/:app_id
# Users with right 'OWNER' on app can access it always
allow = true {
    input.method = "GET"
    input.path = ["api", "mongo", "apps", appId]

    [appId, "OWNER"] = appRights[_]
}

# Path: GET /api/mongo/apps/:app_id
# All apps with 5 stars are public
allow = true {
    some app, appId
    input.method = "GET"
    input.path = ["api", "mongo", "apps", appId]

    data.mongo.apps[app].stars = 5
    app.id = appId
}

# Path: GET /api/mongo/apps/:app_id
# The first app is public
allow = true {
    input.method = "GET"
    input.path = ["api", "mongo", "apps", "1"]
}

# Path: GET <any>
# All users that are a friends of Kevin are allowed see everything
allow = true {
    input.method = "GET"

    # Query
    data.mongo.users[user].name = input.user
    user.friend = "Kevin"
}

appRights[[appId, right]] {
    data.mongo.users[u].name = input.user
    right := data.mongo.app_rights[r].right
    appId := r.app_id
    u.id = r.user_id
}