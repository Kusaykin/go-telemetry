CREATE TABLE IF NOT EXISTS gauges (
    id    bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name  varchar(255) NOT NULL UNIQUE,
    value double precision NOT NULL
);

CREATE TABLE IF NOT EXISTS counters (
    id    bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name  varchar(255) NOT NULL UNIQUE,
    delta bigint NOT NULL
);
