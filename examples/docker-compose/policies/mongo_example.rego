package applications.mongo

verify {
	input.path == ["api", "mongo", "apps", "1"]
}

verify {
	some user

	data.mongo.users[user].name == input.user
	user.password = input.password
}

# Deny all by default
allow := false

# Path: GET /api/mongo/apps/:app_id
# Users with right 'OWNER' on app can access it always
allow {
	some app_id, app, right, user
	input.method == "GET"
	input.path = ["api", "mongo", "apps", app_id]

	# This query fires against collection -> apps
	data.mongo.apps[app].id == app_id

	# Nest elements
	data.mongo.rights[right].right == "OWNER"
	data.mongo.users[user].name == input.user

	# Query root
	app.stars > 2
}

# Path: GET /api/mongo/apps/:app_id
# All apps with 5 stars are public
allow {
	some app, app_id
	input.method == "GET"
	input.path = ["api", "mongo", "apps", app_id]

	# This query fires against collection -> apps
	data.mongo.apps[app].stars == 5
	app.id == app_id
}

# Path: GET /api/mongo/apps/:app_id
# The first app is public
allow {
	input.method == "GET"
	input.path == ["api", "mongo", "apps", "1"]
}

# Path: GET <any>
# All users that are a friends of Kevin are allowed see everything
allow {
	some user
	input.method == "GET"

	# This query fires against collection -> users
	data.mongo.users[user].name == input.user
	old_or_kevin(user.age, user.friend)
}

# Path: GET /api/mongo/apps/:app_id
# Test for count function
allow {
	some app
	input.method == "GET"
	input.path = ["api", "mongo", "apps", "4"]

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
