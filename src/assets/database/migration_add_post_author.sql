-- Migration: Add author field to posts table
-- Run this if you have an existing database

ALTER TABLE posts ADD COLUMN author TEXT;
