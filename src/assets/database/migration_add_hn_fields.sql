-- Migration: Add Hacker News fields to posts table
-- Run this if you have an existing database

ALTER TABLE posts ADD COLUMN url TEXT;
ALTER TABLE posts ADD COLUMN hacker_news_id INTEGER UNIQUE;

CREATE UNIQUE INDEX IF NOT EXISTS idx_posts_hacker_news_id ON posts(hacker_news_id) WHERE hacker_news_id IS NOT NULL;
