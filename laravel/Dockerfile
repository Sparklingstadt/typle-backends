# The Laravel backend for Cloud Run: Laravel Octane on FrankenPHP. Build from the repository root, which holds the data the
# app reads (src/lib/packs, src/messages/ja/format.json); laravel/Dockerfile.dockerignore keeps everything else out of the
# context:
#   docker build -f laravel/Dockerfile -t typle-laravel .
FROM dunglas/frankenphp:1-php8.4
# pdo_pgsql: the database. opcache: the compiled code kept between requests. pcntl: the signals that artisan
# octane:frankenphp passes on to FrankenPHP. ca-certificates: the roots sslmode=verify-full checks Neon's certificate with
# (App\Typle\DatabaseUrl).
RUN install-php-extensions pdo_pgsql opcache pcntl \
    && apt-get update -qq && apt-get install -y --no-install-recommends ca-certificates unzip && rm -rf /var/lib/apt/lists/* \
    && cp "$PHP_INI_DIR/php.ini-production" "$PHP_INI_DIR/php.ini"
COPY laravel/php.ini "$PHP_INI_DIR/conf.d/zz-typle.ini"
COPY --from=composer:2 /usr/bin/composer /usr/bin/composer
ENV APP_ENV=production LOG_CHANNEL=stderr COMPOSER_ALLOW_SUPERUSER=1 TYPLE_APP_DATA=/app-data PORT=8080 OCTANE_WORKERS=5
WORKDIR /app
COPY laravel/composer.json laravel/composer.lock ./
RUN composer install --no-dev --no-scripts --no-autoloader --prefer-dist --no-interaction --no-progress
COPY laravel/ ./
RUN composer dump-autoload --no-dev --classmap-authoritative --no-interaction \
    && rm /usr/bin/composer && mkdir -p storage/logs storage/framework/cache bootstrap/cache
COPY src/lib/packs/samples.json src/lib/packs/countries.json /app-data/src/lib/packs/
COPY src/lib/packs/commands/ /app-data/src/lib/packs/commands/
COPY src/messages/ja/format.json /app-data/src/messages/ja/
EXPOSE 8080
# Five workers, each a PHP thread with the app booted once and its own database connection (as the Rails backend's five
# Puma threads and the pool of five of Next.js). A worker is not restarted after some number of requests (Octane's
# default is 500), so a measurement does not meet a cold worker in its middle.
CMD ["sh", "-c", "exec php artisan octane:frankenphp --host=0.0.0.0 --port=\"$PORT\" --workers=\"$OCTANE_WORKERS\" --max-requests=1000000000 --caddyfile=Caddyfile --log-level=WARN"]
