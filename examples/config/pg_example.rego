package applications.pg

# Deny all by default
allow = false

# Path: GET /api/pg/apps/:app_id
# Users with right 'OWNER' on app can access it always
allow {
    input.method = "GET"
    input.path = ["api", "pg", "apps", appId]

    [appId, "OWNER"] = appRights[_]
}

appRights[[appId, right]] {
    data.pg.users[u].name = input.user
    right := data.pg.app_rights[r].right
    appId := r.app_id
    u.id = r.user_id
}