package applications.mongo

# Deny all by default
allow = false

# Path: GET /api/mongo/apps/:app_id
# Users with right 'OWNER' on app can access it always
allow = true {
    some appId, app, right, user
    input.method == "GET"
    input.path = ["api", "mongo", "apps", appId]

    # This query fires against collection -> apps
    data.mongo.apps[app].id == appId

    # Nest elements
    data.mongo.rights[right].right == "OWNER"
    data.mongo.users[user].name == input.user

    # Query root
    app.stars > 2
}

# Path: GET /api/mongo/apps/:app_id
# All apps with 5 stars are public
allow = true {
    some app, appId
    input.method == "GET"
    input.path = ["api", "mongo", "apps", appId]

    # This query fires against collection -> apps
    data.mongo.apps[app].stars == 5
    app.id == appId
}

# Path: GET /api/mongo/apps/:app_id
# The first app is public
allow = true {
    input.method == "GET"
    input.path == ["api", "mongo", "apps", "1"]
}

# Path: GET <any>
# All users that are a friends of Kevin are allowed see everything
allow = true {
    some user
    input.method == "GET"

    # This query fires against collection -> users
    data.mongo.users[user].name == input.user
    user.friend == "Kevin"
}