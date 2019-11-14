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

db.apps.insert({_id:{ $oid: "5dcd2f2726f56c42f1dfb7e6" },id:2,name:"Arnold's App",stars:3,rights:{right:"OWNER",user:{id:1,name:"Arnold",age:72,friend:"John Connor"}}});
db.apps.insert({_id:{ $oid: "5dcd2fe426f56c42f1dfb7e8" },id:1,name:"First App for everyone",stars:1});
db.apps.insert({_id:{ $oid: "5dcd2ffa26f56c42f1dfb7ea" },id:3,name:"Famous App",stars:5});

db.users.insert({_id:{ $oid: "5dcd307b26f56c42f1dfb7ec" },id:1,name:"Arnold",age:72,friend:"John Connor"});
db.users.insert({_id:{ $oid: "5dcd30a326f56c42f1dfb7ee" },id:2,name:"Kevin",age:21,friend:"Kevin"});
db.users.insert({_id:{ $oid: "5dcd30bc26f56c42f1dfb7f0" },id:3,name:"Anyone",friend:"Anyone"});