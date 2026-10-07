-- Reverts TEC-473. Data loss: which fleet a fleet user belongs to (the
-- users, their fleet role and fleet_profiles.primary_user_id stay).
DROP TABLE IF EXISTS fleet_users;
