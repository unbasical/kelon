package applications.spice

import future.keywords.in

# Deny all by default
default allow = false

verify = true {
    input.path == ["api", "spice", "apps", "1"]
}

verify = true {
    some user

    data.mysql.users[user].name == input.user
    user.password = input.password
}

# Path: GET /api/spice/apps/:app_id
# The first app is public
allow = true {
    input.method == "GET"
    input.path == ["api", "spice", "apps", "1"]
}

# Path: GET /api/spice/apps/:app_id
# App is only accessable by owner and appstore manager
allow = true {
    input.method == "GET"
    input.path = ["api", "spice", "apps", appId]

    subject = sprintf("user:%s", [lower(input.user)])
    resource = sprintf("app:app%s", [appId])

    spice.permission_check(subject, "owner", resource)
}

# Path: PUT /api/spice/apps/:app_id
# Update App only permitted to owner
allow = true {
    input.method == "PUT"
    input.path = ["api", "spice", "apps", appId]

    subject = sprintf("user:%s", [lower(input.user)])
    resource = sprintf("app:app%s", [appId])

    spice.permission_check(subject, "modify", resource)
}

# Path: DELETE /api/spice/apps/:app_id
# App only deletable by owner and appstore manager
allow = true {
    input.method == "DELETE"
    input.path = ["api", "spice", "apps", appId]

    subject = sprintf("user:%s", [lower(input.user)])
    thisApp = sprintf("app%s", [appId])

    apps = spice.lookup_resources(subject, "delete", "app")

    thisApp in apps
}


