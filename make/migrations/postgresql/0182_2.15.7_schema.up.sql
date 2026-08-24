/*
Convert the robot account ID columns to bigint to avoid running out of the int4 range, issue #23091.
*/
ALTER TABLE robot ALTER COLUMN id TYPE bigint;
ALTER TABLE robot ALTER COLUMN creator_ref TYPE bigint;
ALTER TABLE role_permission ALTER COLUMN role_id TYPE bigint;
ALTER SEQUENCE robot_id_seq AS bigint MAXVALUE 9007199254740991;
