package applications.pg

verify if {
	input.path == ["api", "pg", "apps", "1"]
}

verify if {
	some user
	data.pg.pg_users[user].name == input.user
	user.password = input.password
}

# Deny all by default
allow := false

# Path: GET /api/pg/apps/:app_id
# Users with right 'OWNER' on app can access it always
allow if {
	input.method == "GET"
	input.path = ["api", "pg", "apps", app_id]

	# Join
	some u, r
	data.pg.pg_users[u].id == data.pg.pg_app_rights[r].user_id

	# Where
	u.name == input.user
	r.right == "OWNER"
	r.app_id == app_id
}

# Path: GET /api/pg/apps/:app_id
# All apps with 5 stars are public
allow if {
	input.method == "GET"
	input.path = ["api", "pg", "apps", app_id]

	some app
	data.pg.pg_apps[app].id == app_id
	absolute(app.stars) == 5
}

# Path: GET /api/pg/apps/:app_id
# The first app is public
allow if {
	input.method == "GET"
	input.path == ["api", "pg", "apps", "1"]
}

# Path: GET <any>
# All users that are a friends of Kevin are allowed see everything
allow if {
	input.method == "GET"

	# Query
	some user
	data.pg.pg_users[user].name == input.user
	old_or_kevin(user.age, user.friend)
}

# Path: GET /api/pg/apps/:app_id
# Test for count function
allow if {
	input.method == "GET"
	input.path = ["api", "pg", "apps", "4"]

	# Get all apps with 5 starts
	some app
	data.pg.pg_apps[app].stars > 4

	# If there is any one return true
	count(app) > 0
}

old_or_kevin(42, friend)
old_or_kevin(age, "Kevin")
