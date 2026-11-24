CREATE TABLE game.players (
    id text PRIMARY KEY,
    town_hall integer NOT NULL DEFAULT 1 CHECK (town_hall >= 1),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE game.scenario_versions (
    id text NOT NULL,
    version integer NOT NULL CHECK (version > 0),
    definition jsonb NOT NULL,
    checksum text NOT NULL,
    PRIMARY KEY (id, version)
);

CREATE TABLE game.sessions (
    id uuid PRIMARY KEY,
    player_id text NOT NULL REFERENCES game.players(id),
    scenario_id text NOT NULL,
    scenario_version integer NOT NULL,
    state text NOT NULL CHECK (state IN ('active','completed')),
    started_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL CHECK (expires_at > started_at),
    completed_at timestamptz,
    completion_reason text CHECK (completion_reason IN ('time_elapsed','restarted')),
    result jsonb,
    FOREIGN KEY (scenario_id,scenario_version) REFERENCES game.scenario_versions(id,version),
    UNIQUE (id,player_id),
    CHECK ((state='active' AND completed_at IS NULL AND completion_reason IS NULL AND result IS NULL)
        OR (state='completed' AND completed_at IS NOT NULL AND completion_reason IS NOT NULL AND result IS NOT NULL))
);
CREATE UNIQUE INDEX one_active_scenario_per_player ON game.sessions(player_id,scenario_id) WHERE state='active';
CREATE INDEX due_sessions ON game.sessions(expires_at,id) WHERE state='active';
CREATE INDEX player_session_history ON game.sessions(player_id,started_at DESC,id DESC);

CREATE TABLE game.session_events (
    session_id uuid NOT NULL REFERENCES game.sessions(id),
    sequence bigint NOT NULL CHECK (sequence > 0),
    kind text NOT NULL,
    effective_at timestamptz NOT NULL,
    recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    payload jsonb NOT NULL,
    PRIMARY KEY (session_id,sequence)
);

CREATE TABLE game.command_receipts (
    player_id text NOT NULL REFERENCES game.players(id),
    command_key text NOT NULL,
    request_hash text NOT NULL,
    response jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (player_id,command_key)
);

CREATE TABLE trading.accounts (
    id uuid PRIMARY KEY,
    session_id uuid NOT NULL UNIQUE,
    player_id text NOT NULL,
    currency text NOT NULL CHECK (currency='USD'),
    created_at timestamptz NOT NULL,
    FOREIGN KEY (session_id,player_id) REFERENCES game.sessions(id,player_id)
);
CREATE TABLE trading.journals (
    id uuid PRIMARY KEY,
    account_id uuid NOT NULL REFERENCES trading.accounts(id),
    kind text NOT NULL CHECK (kind='DEMO_ALLOCATION'),
    effective_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE (account_id,kind)
);
CREATE TABLE trading.ledger_entries (
    journal_id uuid NOT NULL REFERENCES trading.journals(id),
    bucket text NOT NULL CHECK (bucket IN ('player_cash','demo_source')),
    amount_minor bigint NOT NULL CHECK (amount_minor<>0),
    PRIMARY KEY (journal_id,bucket)
);

CREATE FUNCTION trading.reject_record_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'immutable record cannot be changed' USING ERRCODE='23514';
END;
$$;
CREATE TRIGGER immutable_journals BEFORE UPDATE OR DELETE ON trading.journals
    FOR EACH ROW EXECUTE FUNCTION trading.reject_record_change();
CREATE TRIGGER immutable_ledger BEFORE UPDATE OR DELETE ON trading.ledger_entries
    FOR EACH ROW EXECUTE FUNCTION trading.reject_record_change();
CREATE TRIGGER immutable_session_events BEFORE UPDATE OR DELETE ON game.session_events
    FOR EACH ROW EXECUTE FUNCTION trading.reject_record_change();
CREATE TRIGGER immutable_scenario_versions BEFORE UPDATE OR DELETE ON game.scenario_versions
    FOR EACH ROW EXECUTE FUNCTION trading.reject_record_change();

CREATE FUNCTION trading.require_balanced_journal() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    target uuid;
    entry_count bigint;
    total numeric;
BEGIN
    IF TG_TABLE_NAME='journals' THEN target := NEW.id; ELSE target := NEW.journal_id; END IF;
    SELECT count(*),COALESCE(sum(amount_minor),0) INTO entry_count,total
        FROM trading.ledger_entries WHERE journal_id=target;
    IF entry_count<>2 OR total<>0 THEN
        RAISE EXCEPTION 'journal must have two balanced entries' USING ERRCODE='23514';
    END IF;
    RETURN NULL;
END;
$$;
CREATE CONSTRAINT TRIGGER balanced_journal AFTER INSERT ON trading.journals
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION trading.require_balanced_journal();
CREATE CONSTRAINT TRIGGER balanced_entries AFTER INSERT ON trading.ledger_entries
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION trading.require_balanced_journal();
