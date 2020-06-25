DROP SCHEMA IF EXISTS "appstore";
CREATE SCHEMA "appstore";

CREATE SEQUENCE apps_id_seq INCREMENT 1 MINVALUE 1 MAXVALUE 2147483647 START 1 CACHE 1;
CREATE TABLE "appstore"."apps" (
    "id" integer DEFAULT nextval('apps_id_seq') NOT NULL,
    "name" character varying(255) NOT NULL,
    "stars" smallint,
    CONSTRAINT "apps_id" PRIMARY KEY ("id")
) WITH (oids = false);

INSERT INTO "appstore"."apps" ("id", "name", "stars") VALUES
(1,	'First App for everyone',	1),
(2,	'Arnold''s App',	3),
(3,	'Famous App',	5);

CREATE SEQUENCE users_id_seq INCREMENT 1 MINVALUE 1 MAXVALUE 2147483647 START 1 CACHE 1;
CREATE TABLE "appstore"."users" (
    "id" integer DEFAULT nextval('users_id_seq') NOT NULL,
    "name" character varying(255) NOT NULL,
    "age" smallint,
    "friend" character varying,
    CONSTRAINT "users_id" PRIMARY KEY ("id")
) WITH (oids = false);

INSERT INTO "appstore"."users" ("id", "name", "age", "friend") VALUES
(1,	'Arnold',	72,	'John Connor'),
(2,	'Kevin',	21,	'Kevin'),
(3,	'Anyone',	NULL,	'Anyone');


CREATE TABLE "appstore"."app_rights" (
    "app_id" bigint NOT NULL,
    "user_id" bigint NOT NULL,
    "right" character varying NOT NULL,
    CONSTRAINT "app_rights_app_id_user_id_right" PRIMARY KEY ("app_id", "user_id", "right"),
    CONSTRAINT "app_rights_app_id_fkey" FOREIGN KEY (app_id) REFERENCES appstore.apps(id) NOT DEFERRABLE,
    CONSTRAINT "app_rights_user_id_fkey" FOREIGN KEY (user_id) REFERENCES appstore.users(id) NOT DEFERRABLE
) WITH (oids = false);

INSERT INTO "appstore"."app_rights" ("app_id", "user_id", "right") VALUES
(2,	1,	'OWNER');