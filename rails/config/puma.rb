# One process of five threads, as many as the connections of the pool (database.yml) and of Next.js's pg pool.
threads_count = ENV.fetch("RAILS_MAX_THREADS", 5)
threads threads_count, threads_count
port ENV.fetch("PORT", 3000)
pidfile ENV["PIDFILE"] if ENV["PIDFILE"]
