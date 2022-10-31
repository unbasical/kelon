db.createUser(
    {
        user   : "You",
        pwd    : "SuperSecure",
        roles  : [
            {
                role  : "readWrite",
                db    : "appstore"
            }
        ]
    }
);

db.apps.insertOne({id:2,name:"Arnold's App",stars:3,rights:[{right:"OWNER",user:{id:1,name:"Arnold",age:72,friend:"John Connor"}}]});
db.apps.insertOne({id:1,name:"First App for everyone",stars:1});
db.apps.insertOne({id:3,name:"Famous App",stars:5});

db.users.insertOne({id:1,name:"Arnold",age:72,friend:"John Connor"});
db.users.insertOne({id:2,name:"Kevin",age:21,friend:"Kevin"});
db.users.insertOne({id:3,name:"Anyone",friend:"Anyone"});
db.users.insertOne({id:4,name:"Torben",age:42,friend:"Daniel"});