-- Schema for Kill the Newsletter! (Go)
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS feeds (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  publicId TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL,
  icon TEXT NULL,
  emailIcon TEXT NULL
);
CREATE INDEX IF NOT EXISTS index_feeds_publicId ON feeds(publicId);

CREATE TABLE IF NOT EXISTS feedEntries (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  publicId TEXT NOT NULL UNIQUE,
  feed INTEGER NOT NULL REFERENCES feeds(id) ON DELETE CASCADE,
  createdAt TEXT NOT NULL,
  author TEXT NULL,
  title TEXT NOT NULL,
  content TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS index_feedEntries_publicId ON feedEntries(publicId);
CREATE INDEX IF NOT EXISTS index_feedEntries_feed ON feedEntries(feed);

CREATE TABLE IF NOT EXISTS feedEntryEnclosures (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  publicId TEXT NOT NULL UNIQUE,
  type TEXT NOT NULL,
  length INTEGER NOT NULL,
  name TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS feedEntryEnclosureLinks (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  feedEntry INTEGER NOT NULL REFERENCES feedEntries(id) ON DELETE CASCADE,
  feedEntryEnclosure INTEGER NOT NULL REFERENCES feedEntryEnclosures(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS index_feedEntryEnclosureLinks_feedEntry ON feedEntryEnclosureLinks(feedEntry);
CREATE INDEX IF NOT EXISTS index_feedEntryEnclosureLinks_feedEntryEnclosure ON feedEntryEnclosureLinks(feedEntryEnclosure);

CREATE TABLE IF NOT EXISTS feedVisualizations (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  feed INTEGER NOT NULL REFERENCES feeds(id) ON DELETE CASCADE,
  createdAt TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS index_feedVisualizations_feed ON feedVisualizations(feed);
CREATE INDEX IF NOT EXISTS index_feedVisualizations_createdAt ON feedVisualizations(createdAt);

CREATE TABLE IF NOT EXISTS feedWebSubSubscriptions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  feed INTEGER NOT NULL REFERENCES feeds(id) ON DELETE CASCADE,
  createdAt TEXT NOT NULL,
  callback TEXT NOT NULL,
  secret TEXT NULL,
  UNIQUE(feed, callback)
);
CREATE INDEX IF NOT EXISTS index_feedWebSubSubscriptions_feed ON feedWebSubSubscriptions(feed);
CREATE INDEX IF NOT EXISTS index_feedWebSubSubscriptions_createdAt ON feedWebSubSubscriptions(createdAt);
CREATE INDEX IF NOT EXISTS index_feedWebSubSubscriptions_callback ON feedWebSubSubscriptions(callback);

-- Background jobs table
CREATE TABLE IF NOT EXISTS backgroundJobs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  type TEXT NOT NULL,
  startAt TEXT NOT NULL,
  parameters TEXT NOT NULL,
  retries INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'pending'
);
CREATE INDEX IF NOT EXISTS index_backgroundJobs_type ON backgroundJobs(type);
CREATE INDEX IF NOT EXISTS index_backgroundJobs_startAt ON backgroundJobs(startAt);
CREATE INDEX IF NOT EXISTS index_backgroundJobs_status ON backgroundJobs(status);
