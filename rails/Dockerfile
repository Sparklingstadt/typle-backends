# The Rails backend for Cloud Run. Build from the repository root, which holds the data the app reads (src/lib/packs,
# src/messages/ja/format.json); rails/Dockerfile.dockerignore keeps everything else out of the context:
#   docker build -f rails/Dockerfile -t typle-rails .
FROM ruby:3.4.1-slim AS build
# The compilers of the gems with native extensions (bootsnap, msgpack, date, psych...), in the build stage only.
RUN apt-get update -qq && apt-get install -y --no-install-recommends build-essential libyaml-dev pkg-config \
    && rm -rf /var/lib/apt/lists/*
ENV BUNDLE_DEPLOYMENT=1 BUNDLE_WITHOUT=test BUNDLE_PATH=/usr/local/bundle
WORKDIR /app
COPY rails/Gemfile rails/Gemfile.lock rails/.ruby-version ./
RUN bundle install && rm -rf "$BUNDLE_PATH"/ruby/*/cache
COPY rails/ ./
RUN bundle exec bootsnap precompile app/ config/

FROM ruby:3.4.1-slim
# ca-certificates: the roots sslmode=verify-full checks Neon's certificate with (Typle::DatabaseUrl).
RUN apt-get update -qq && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
ENV RAILS_ENV=production BUNDLE_DEPLOYMENT=1 BUNDLE_WITHOUT=test BUNDLE_PATH=/usr/local/bundle \
    TYPLE_APP_DATA=/app-data PORT=8080
COPY --from=build /usr/local/bundle /usr/local/bundle
COPY --from=build /app /app
COPY src/lib/packs/samples.json src/lib/packs/countries.json /app-data/src/lib/packs/
COPY src/lib/packs/commands/ /app-data/src/lib/packs/commands/
COPY src/messages/ja/format.json /app-data/src/messages/ja/
WORKDIR /app
EXPOSE 8080
# Puma: one process of five threads (config/puma.rb); YJIT is on (Rails' default since 7.2).
CMD ["bin/rails", "server", "-b", "0.0.0.0"]
