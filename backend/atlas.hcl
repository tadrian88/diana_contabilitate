env "local" {
  url = getenv("DATABASE_URL")
  migration {
    dir = "file://migrations"
  }
}

env "test" {
  url = getenv("TEST_DATABASE_URL")
  migration {
    dir = "file://migrations"
  }
}

// Cloud releases use a one-shot Cloud Run Job. API and worker revisions never
// apply migrations during startup.
env "cloud" {
  url = getenv("DATABASE_URL")
  migration {
    dir = "file://migrations"
  }
}
