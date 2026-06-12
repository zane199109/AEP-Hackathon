-- AEP 数据库初始化脚本
-- PostgreSQL 15+
-- 使用: psql $DSN -f schema.sql

-- 核心赏金表
CREATE TABLE IF NOT EXISTS bounties (
    id              BIGSERIAL PRIMARY KEY,
    job_id          BIGINT NOT NULL UNIQUE,          -- 链上 jobId
    buyer_addr      BYTEA NOT NULL,                  -- 买家地址 (20 bytes)
    seller_addr     BYTEA DEFAULT NULL,              -- 卖家地址 (20 bytes)
    amount          NUMERIC(78, 0) NOT NULL,         -- 锁定金额 (wei)
    deadline        TIMESTAMPTZ NOT NULL,            -- 截止时间
    result_hash     TEXT DEFAULT '',                 -- IPFS 交付物哈希
    status          VARCHAR(20) NOT NULL DEFAULT 'Open',  -- Open|Assigned|Submitted|Verified|Slashed|Refunded
    pact_id         VARCHAR(255) DEFAULT '',         -- CAW Pact ID
    buyer_approval  BOOLEAN NOT NULL DEFAULT FALSE,  -- 买方确认放行 (红线: 结算前必须校验)
    -- 委托链字段 (Day 1: sub-bounty support)
    parent_bounty_id BIGINT DEFAULT NULL REFERENCES bounties(job_id),
    depth           INTEGER NOT NULL DEFAULT 0,      -- 委托深度 (0=根任务)
    buyer_wallet_id VARCHAR(255) DEFAULT '',         -- 买方 CAW 钱包 ID
    seller_wallet_id VARCHAR(255) DEFAULT '',        -- 卖方 CAW 钱包 ID
    rule_params     JSONB DEFAULT '[]'::jsonb,      -- LLM 提取的规则参数 (模板化 Rule 引擎)
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_bounties_status ON bounties(status);
CREATE INDEX idx_bounties_buyer ON bounties(buyer_addr);
CREATE INDEX idx_bounties_pact_id ON bounties(pact_id);

-- L2 幂等防重表 (联合主键)
CREATE TABLE IF NOT EXISTS processed_events (
    tx_hash     BYTEA NOT NULL,           -- 交易哈希 (32 bytes)
    log_index   INTEGER NOT NULL,         -- 事件日志索引
    event_type  VARCHAR(50) NOT NULL,     -- 事件类型
    job_id      BIGINT NOT NULL,          -- 关联 jobId
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tx_hash, log_index)
);

CREATE INDEX idx_processed_events_job_id ON processed_events(job_id);
