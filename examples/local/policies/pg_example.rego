package applications.pg

verify = true {
    input.path == ["api", "pg", "apps", "1"]
}

verify = true {
    some user

    data.pg.pg_users[user].name == input.user
    user.password = input.password
}

# Deny all by default
allow = false

# Path: GET /api/pg/apps/:app_id
# Users with right 'OWNER' on app can access it always
allow = true {
    some appId, u, r
    input.method == "GET"
    input.path = ["api", "pg", "apps", appId]

    # Join
    data.pg.pg_users[u].id == data.pg.pg_app_rights[r].user_id

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

    data.pg.pg_apps[app].id == appId
    absolute(app.stars) == 5
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
    data.pg.pg_users[user].name == input.user
    old_or_kevin(user.age, user.friend)
}

# Path: GET /api/pg/apps/:app_id
# Test for count function
allow = true {
    some app
    input.method == "GET"
    input.path = ["api", "pg", "apps", "4"]

    # Get all apps with 5 starts
    data.pg.pg_apps[app].stars > 4

    #If there is any one return true
    count(app) > 0
}

old_or_kevin(age, friend) {
    age == 42
}

old_or_kevin(age, friend) {
    friend == "Kevin"
}
