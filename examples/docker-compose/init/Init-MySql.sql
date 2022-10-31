-- Adminer 4.7.3 MySQL dump

SET NAMES utf8;
SET time_zone = '+00:00';
SET foreign_key_checks = 0;
SET sql_mode = 'NO_AUTO_VALUE_ON_ZERO';

DROP DATABASE IF EXISTS `appstore`;
CREATE DATABASE `appstore` /*!40100 DEFAULT CHARACTER SET latin1 */;
USE `appstore`;

DROP TABLE IF EXISTS `apps`;
CREATE TABLE `apps` (
  `id` int(11) NOT NULL AUTO_INCREMENT,
  `name` varchar(255) NOT NULL,
  `stars` tinyint(3) unsigned zerofill NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=latin1;

INSERT INTO `apps` (`id`, `name`, `stars`) VALUES
(1,	'First App for everyone',	001),
(2,	'Arnold\'s App',	003),
(3,	'Famous App',	005);

DROP TABLE IF EXISTS `app_rights`;
CREATE TABLE `app_rights` (
  `app_id` int(11) NOT NULL,
  `user_id` int(11) NOT NULL,
  `right` varchar(255) NOT NULL,
  KEY `app_id` (`app_id`),
  KEY `user_id` (`user_id`),
  CONSTRAINT `app_rights_ibfk_1` FOREIGN KEY (`app_id`) REFERENCES `apps` (`id`),
  CONSTRAINT `app_rights_ibfk_2` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=latin1;

INSERT INTO `app_rights` (`app_id`, `user_id`, `right`) VALUES
(2,	1,	'OWNER');

DROP TABLE IF EXISTS `users`;
CREATE TABLE `users` (
  `id` int(11) NOT NULL AUTO_INCREMENT,
  `name` varchar(255) NOT NULL,
  `age` tinyint(4) DEFAULT NULL,
  `friend` varchar(255) NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=latin1;

INSERT INTO `users` (`id`, `name`, `age`, `friend`) VALUES
(1,	'Arnold',	72,	'John Connor'),
(2,	'Kevin',	21,	'Kevin'),
(3,	'Anyone',	NULL,	'Anyone'),
(4, 'Torben', 42, 'Daniel');