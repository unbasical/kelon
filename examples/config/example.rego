package example

# Path: GET /apps/:app_id
# Users with right 'ENGINEER' can access apps with tag 'TECH' with more than 4 stars
allow = true {
    some app
    input.method == "GET"
    input.path = ["apps", post_id]

    [app.id, "ENGINEER"] = appRights[_]
    data.apps[app].stars >= 4
}

allow = true {
    some id
    input.method == "GET"
    input.path = ["apps", id]
    id == 1
}

allow = true {
    input.method == "GET"

    # Query
    data.users[user].name == input.user
    user.friend == "Kevin"
}

appRights[[app, right]] {
    data.users[u].name == input.user
    right := data.app_rights[r].right
    app := r.app_id
    u.id == r.user_id
}

appTags[[app, tag]] {
    tag := data.app_tags[t].tag
    app := r.app_id
    u.id == r.user_id
}

isTokenUser[user] {
    data.users[user].name = input.user
}