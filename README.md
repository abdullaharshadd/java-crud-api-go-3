# java-crud-api (Go port)

A REST API for creating, reading, updating and deleting **users**. This branch is a migration of [`abdullaharshadd/java-crud-api`](https://github.com/abdullaharshadd/java-crud-api) from Java/Spring Boot to Go using the standard library.

> **Migration status: incomplete and only partly verified.**
>
> | Check | Status |
> |---|---|
> | Build | ✅ Passed |
> | Unit tests | ⚠️ Not run |
> | Behavior compared with the original Spring app | ⚠️ Not run |
> | Modules migrated | 7 of 8 |
> | Migration tool's own confidence estimate | 82% (estimated by the tool, not measured) |
>
> The code compiles. Nobody has confirmed that it behaves like the original. Read [Known limitations](#known-limitations) and [Manual review required](#manual-review-required) before you rely on it.

---

## Tech stack

| Concern | Original | This branch |
|---|---|---|
| Language | Java | Go (version declared in `go.mod`) |
| HTTP layer | Spring MVC | Go standard library (`cmd/server/router.go`, `internal/httpapi/`) |
| Persistence | Spring Data JPA / Hibernate | Hand-written repository (`internal/store/user_repository.go`) |
| Validation | Bean Validation (`@NotBlank`) | Explicit Go code |
| Models | Lombok-annotated classes | Plain Go structs (`internal/model/`) |
| Dependencies | Maven/Gradle | Go modules (`go.mod`) |

`go.mod` is the authoritative list of third-party modules, including the database driver.

---

## Prerequisites

- A Go toolchain that matches the `go` directive in `go.mod`.
- `sh`, `find`, `grep`, `xargs` and `dirname`. The install command depends on all of them.
- Write access to `/app/bin`. The install command builds the binary to `/app/bin/server`.
- A reachable database instance for `DATABASE_URL`. To find out which database engine is expected, check the driver imported in `go.mod` and `internal/store/user_repository.go`.

---

## Getting started

Run these steps in order, starting from a fresh clone.

### 1. Install dependencies and build

```sh
sh -c 'ROOT=$(dirname "$(find "$PWD" /app /workdir -maxdepth 3 -name go.mod -not -path "*/vendor/*" 2>/dev/null | head -1)"); cd "$ROOT" && MAIN=$(grep -rl --include=*.go "^package main" . | grep -v _test.go | grep -v /vendor/ | head -1 | xargs dirname) && echo "root=$ROOT main=$MAIN" && go mod tidy && go mod download && mkdir -p /app/bin && go build -o /app/bin/server "$MAIN"'
```

This command does the following:

1. Finds the module root.
2. Finds the `main` package, which should resolve to `cmd/server`.
3. Resolves and downloads the dependencies.
4. Builds the binary to `/app/bin/server`.

### 2. Resolve module dependencies

This step ran successfully during the migration. Run it from the repository root, where `go.mod` lives:

```sh
go mod tidy
```

### 3. Set environment variables

```sh
export DATABASE_URL="<your database connection string>"
export PORT="<port to listen on>"
export JWT_SECRET="<secret value>"
```

See [Environment variables](#environment-variables) for details.

### 4. Set up the database

The migration did not detect a database setup command. The original app used Hibernate `ddl-auto` to create the `USER` table automatically when it started. Go has no equivalent of that mechanism, as described under [Known limitations](#known-limitations).

Before the first run, do the following:

1. Check `internal/store/user_repository.go` and `cmd/server/main.go` to see whether either one creates the schema at startup.
2. If neither does, create the users table by hand in the database that `DATABASE_URL` points to.
   - Use the table name, column names, types and lengths that the queries in `internal/store/user_repository.go` expect.
   - Include the unique constraint that the original entity declared.
   - `USER` is a reserved word in many databases, so it may need quoting.

### 5. Run

```sh
/app/bin/server
```

---

## Running tests

```sh
go test ./...
```

Tests have **not** been run on this branch. The project file list contains no test files. This command will probably report that no tests exist rather than confirm that the code is correct. Adding tests for `internal/service/user_service.go` and `internal/httpapi/handler.go` is recommended.

---

## Environment variables

These variables were detected as required. For the exact parsing, defaults and validation, see `internal/config/config.go`.

| Variable | Purpose | Notes |
|---|---|---|
| `DATABASE_URL` | Connection string for the user database | Its format depends on the driver in `go.mod` |
| `PORT` | Port the HTTP server listens on | Check `internal/config/config.go` for any default |
| `JWT_SECRET` | Secret used for JWT handling | Check where this is used. No auth module appears in the file list. |

---

## Architecture overview

```
cmd/server/
  main.go              Entry point: loads config, wires dependencies, starts the HTTP server
  router.go            Route registration (maps HTTP paths/methods to handlers)
internal/config/
  config.go            Reads environment variables into a config struct
internal/httpapi/
  handler.go           HTTP handlers for the user endpoints (replaces the Spring controller)
  errors.go            Maps errors to HTTP responses (replaces the Spring exception handler)
internal/service/
  user_service.go      Business logic for users (replaces UserServiceImp)
  errors.go            Service-level error values (e.g. "user not found")
internal/store/
  user_repository.go   Explicit SQL data access for users (replaces the JPA UserDao)
internal/model/
  user.go              User struct (replaces the Lombok/JPA User entity)
  error_message.go     Error response body shape
```

A request moves through the layers in this order:

```
router.go → httpapi/handler.go → service/user_service.go → store/user_repository.go → database
```

Errors travel back up the same path. `internal/httpapi/errors.go` converts them into HTTP responses.

The descriptions above are based on file names and the migration mapping. Read each file to confirm its exact responsibilities.

---

## Migration notes

These are the main changes from the Spring codebase:

- **Dependency injection.** Spring's automatic wiring has been replaced by explicit construction in `cmd/server/main.go`.
- **Controllers.** Annotated Spring controllers are now standard-library HTTP handlers (`internal/httpapi/handler.go`). Their routes are registered in `cmd/server/router.go`.
- **Exception handling.**
  - The Spring `@ControllerAdvice`-style handler (`RestResponseEntityExceptionHandling`) has been replaced by error-to-response mapping in `internal/httpapi/errors.go`.
  - Exceptions such as `UserNotFoundException` have become Go error values in `internal/service/errors.go`.
- **Repository.** Spring Data's generated `UserDao` implementation has been replaced by the hand-written `internal/store/user_repository.go`.
  - Spring Data generated queries from method names. That no longer happens: every query is explicit.
  - JPA persistence-context behavior no longer applies. That includes dirty checking, lazy loading and cascades.
- **Model.**
  - Lombok-generated getters, setters, builders and constructors have been replaced by a plain struct in `internal/model/user.go`.
  - The original `toString`/serialization excluded the password. Confirm that the JSON output does the same.
- **Validation.** `@NotBlank` annotations no longer run automatically. Equivalent checks must be written explicitly in Go and must reject values that are blank once trimmed.
- **Coverage.** 7 of 8 modules were migrated. The migration output does not say which module is missing. Compare against the original repository to find out.

---

## Known limitations

These parts of the original could not be migrated directly:

| Original component | Reason | Approach in Go |
|---|---|---|
| `UserNotFoundException(String, Throwable, boolean, boolean)` constructor | `enableSuppression` and `writableStackTrace` are internals of the JVM `Throwable` class | Omitted. Go error values are lightweight already. |
| Hibernate `ddl-auto` schema generation for `User` | Hibernate created the tables from annotations at startup. Go has no implicit equivalent. | The schema must be created explicitly. See [step 4](#4-set-up-the-database). |
| Lombok annotations (`@Data`, `@Builder`, `@NoArgsConstructor`, `@AllArgsConstructor`) | Java-only compile-time code generation | Replaced by a plain Go struct |
| `@NotBlank` Bean Validation | Ran through Hibernate Validator and Spring's `@Valid` | Must be explicit Go validation that keeps the same error messages |
| `UserDao` (JpaRepository runtime proxy and query derivation) | Spring generated the implementation at runtime, so no source code existed to translate | Rewritten by hand in `internal/store/user_repository.go` |
| JPA persistence-context semantics (lazy loading, dirty checking, cascades) | Hibernate runtime behavior with no Go standard-library equivalent | Must be explicit in the service layer, inside transactions where needed |

Other limitations:

- One of the eight modules has not been migrated.
- No tests exist or have been run.
- No comparison against the original's behavior has been done.
- There is no detected database setup step.
- `JWT_SECRET` is listed as required, but no authentication-specific file appears in the project. Its use, if any, needs to be confirmed.

---

## Manual review required

### Low-confidence migrations (highest priority)

1. **`RestResponseEntityExceptionHandling` (original error handler)**
   - Review its Go replacement in `internal/httpapi/errors.go`, together with `internal/model/error_message.go` and `internal/service/errors.go`.
   - For each error case, confirm that the HTTP status codes and response body shapes match the original. Cover not-found errors, validation failures and malformed request bodies.
2. **`UserServiceImp` (original service implementation)**
   - Review `internal/service/user_service.go`.
   - Confirm the behavior of every CRUD method, especially:
     - how updates merge fields;
     - what happens when a user does not exist;
     - how uniqueness conflicts are handled.

### Components affected by the unmigrable items

3. **`internal/store/user_repository.go`**
   - Check that the SQL matches the original table and column names.
   - Check that reserved identifiers such as `USER` are quoted if needed.
   - Check that duplicate-key errors from the unique constraint are handled.
4. **`internal/model/user.go`**
   - Check that the password is never written to JSON responses.
   - Check that the field names match the original API's JSON contract.
5. **Validation**
   - Find where request validation happens, either in `internal/httpapi/handler.go` or in `internal/service/user_service.go`.
   - Confirm that every field the original marked `@NotBlank` is rejected when it is empty or whitespace-only.
   - Confirm that the original error messages are kept.
6. **Database schema**
   - Confirm how the schema is created, if it is created at all.
   - If nothing creates it, add an explicit migration or startup step.

### Configuration and coverage

7. **`internal/config/config.go`**
   - Check the defaults and the behavior when `DATABASE_URL`, `PORT` or `JWT_SECRET` is missing.
   - Confirm where `JWT_SECRET` is actually used.
8. **Missing module**
   - Identify which of the 8 original modules was not migrated.
   - Decide whether it is still needed.