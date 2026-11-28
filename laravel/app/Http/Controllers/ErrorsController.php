<?php

declare(strict_types=1);

namespace App\Http\Controllers;

use App\Http\Answer;
use Illuminate\Http\Request;

/**
 * The answers of the paths and methods no handler serves: 405 with the methods the path serves, and 404.
 */
class ErrorsController extends Controller
{
    /** The methods each path serves, for the Allow header of its 405. */
    public const ALLOW = [
        'api/health' => 'GET, HEAD',
        'api/workspace' => 'GET, HEAD, PUT',
        'api/workspace/records' => 'POST, DELETE',
    ];

    public function methodNotAllowed(Request $request): Answer
    {
        return self::notAllowed(self::ALLOW[$request->path()]);
    }

    public function notFound(): Answer
    {
        return $this->refuse(404, 'notFound');
    }

    public static function notAllowed(string $allow): Answer
    {
        $answer = Answer::error(405, 'methodNotAllowed');
        $answer->headers->set('allow', $allow);

        return $answer;
    }
}
