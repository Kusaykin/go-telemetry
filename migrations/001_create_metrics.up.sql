CREATE TABLE IF NOT EXISTS gauges (
    name  text PRIMARY KEY,
    value double precision NOT NULL
);

CREATE TABLE IF NOT EXISTS counters (
    name  text PRIMARY KEY,
    delta bigint NOT NULL
);
