package applications.mixed

# Here we mix multiple datastores (MongoDB and Postgres)
# NOTE: Only one datastore can be used in a allow/verify policy

verify {
	input.path == ["api", "mixed", "apps", "1"]
}

# Verify using Postgres as Datastore
verify {
	some user

	data.pg.pg_users[user].name == input.user
	user.password = input.password
}

# Deny all by default
allow := false

# Path: GET /api/pg/apps/:app_id
# Datastore: Postgres
# The first app is public
allow {
	input.method == "GET"
	input.path == ["api", "mixed", "apps", "1"]
}

# Path: GET /api/pg/apps/:app_id
# Datastore: Postgres
# Users with right 'OWNER' on app can access it always
allow {
	some app_id, u, r
	input.method == "GET"
	input.path = ["api", "mixed", "apps", app_id]

	# Join
	data.pg.pg_users[u].id == data.pg.pg_app_rights[r].user_id

	# Where
	u.name == input.user
	r.right == "OWNER"
	r.app_id == app_id
}

# Path: GET /api/pg/apps/:app_id
# Datastore: Postgres
# All apps with 5 stars are public
allow {
	some app, app_id
	input.method == "GET"
	input.path = ["api", "mixed", "apps", app_id]

	data.pg.pg_apps[app].id == app_id
	app.stars == 5
}

# Path: GET <any>
# Datastore: Mongo
# All users that are a friends of Kevin are allowed see everything
allow {
	input.method == "GET"

	# Query
	data.mongo.users[user].name == input.user
	old_or_kevin(user.age, user.friend)
}

# Path: GET /api/pg/apps/:app_id
# Datastore: MongoDB
# Test for count function
allow {
	some app
	input.method == "GET"
	input.path = ["api", "mixed", "apps", "4"]

	# Get all apps with 5 starts
	data.mongo.apps[app].stars > 4

	# If there is any one return true
	count(app) > 0
}

old_or_kevin(age, friend) {
	age == 42
}

old_or_kevin(age, friend) {
	friend == "Kevin"
}
