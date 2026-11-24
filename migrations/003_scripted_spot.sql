ALTER TABLE trading.accounts ADD COLUMN currency_scale integer NOT NULL DEFAULT 2 CHECK (currency_scale IN (2,12));
ALTER TABLE trading.journals ADD COLUMN asset text NOT NULL DEFAULT 'USD' CHECK (asset IN ('USD','XRP','SOL','ETH'));
ALTER TABLE trading.journals ADD COLUMN operation_id uuid;
ALTER TABLE trading.journals DROP CONSTRAINT journals_kind_check;
ALTER TABLE trading.journals DROP CONSTRAINT journals_account_id_kind_key;
ALTER TABLE trading.journals ADD CONSTRAINT journal_kind CHECK (kind IN ('DEMO_ALLOCATION','SPOT_BUY','SPOT_SELL','PERP_PNL'));
CREATE UNIQUE INDEX one_demo_allocation ON trading.journals(account_id) WHERE kind='DEMO_ALLOCATION';
CREATE UNIQUE INDEX one_posting_per_operation_asset ON trading.journals(account_id,operation_id,asset) WHERE operation_id IS NOT NULL;
ALTER TABLE game.sessions ADD COLUMN last_reconciled_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE game.sessions DROP CONSTRAINT sessions_completion_reason_check;
ALTER TABLE game.sessions ADD CONSTRAINT session_completion_reason CHECK (completion_reason IN ('time_elapsed','restarted','finished'));
CREATE TABLE trading.execution_state (
 session_id uuid PRIMARY KEY REFERENCES game.sessions(id),
 state jsonb NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE game.lesson_progress (
 player_id text NOT NULL REFERENCES game.players(id),
 lesson_id text NOT NULL,
 session_id uuid NOT NULL REFERENCES game.sessions(id),
 completed_at timestamptz NOT NULL,
 PRIMARY KEY(player_id,lesson_id)
);
