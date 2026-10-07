-- Reverts TEC-463 user-identity campaign opt-outs.

DROP TABLE IF EXISTS campaign_user_opt_out_state;
DROP TABLE IF EXISTS campaign_user_opt_outs;
DROP FUNCTION IF EXISTS campaign_user_opt_outs_project();
DROP FUNCTION IF EXISTS campaign_user_opt_outs_append_only();
