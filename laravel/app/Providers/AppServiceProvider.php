<?php

namespace App\Providers;

use App\Database\PostgresConnector;
use App\Typle\Report;
use App\Typle\SessionUsers;
use App\Typle\WorkspaceStore;
use Illuminate\Support\ServiceProvider;

class AppServiceProvider extends ServiceProvider
{
    public function register(): void
    {
        // The services the controllers and middleware take, which the tests stand in for.
        $this->app->singleton(WorkspaceStore::class);
        $this->app->singleton(SessionUsers::class);
        $this->app->singleton(Report::class);

        // PostgreSQL connections get the URL's parameters for libpq too (App\Typle\DatabaseUrl).
        $this->app->bind('db.connector.pgsql', PostgresConnector::class);
    }
}
