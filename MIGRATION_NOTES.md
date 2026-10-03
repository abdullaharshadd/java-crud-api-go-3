# Migration Notes

**Model self-assessed confidence:** 82% (not a measured result)  
**Build at time of writing:** passed  
**Target unit tests:** not run  
**Behavior compared with the original:** not run

The final recommendation is in the pull request description.

---

## What was migrated

- `src/main/java/com/smartContact/error/UserNotFoundException.java` → `internal/service/errors.go` (89% confidence)
- `src/main/java/com/smartContact/model/ErrorMessage.java` → `internal/model/error_message.go` (89% confidence)
- `src/main/java/com/smartContact/repository/UserDao.java` → `internal/store/user_repository.go` (85% confidence)
- `src/main/java/com/smartContact/error/RestResponseEntityExceptionHandling.java` → `internal/httpapi/errors.go` (84% confidence) ⚠️ needs review
- `src/main/java/com/smartContact/service/UserService.java` → `internal/service/user_service.go` (86% confidence)
- `src/main/java/com/smartContact/service/UserServiceImp.java` → `internal/service/user_service.go` (72% confidence) ⚠️ needs review
- `src/main/java/com/smartContact/Controller/UserController.java` → `internal/httpapi/handler.go` (88% confidence)

## Not migrated, or migrated with open issues

- `src/main/java/com/smartContact/model/User.java`: ModuleStatus.completed_with_open_issues

## Components that could not be automatically migrated

These components require manual implementation. The migrated code contains
`MIGRATION_NOTE` comments at the relevant locations.

### `UserNotFoundException(String, Throwable, boolean, boolean)` in `src/main/java/com/smartContact/error/UserNotFoundException.java`
**Reason:** The enableSuppression and writableStackTrace flags are JVM Throwable internals with no direct equivalent in most target languages.
**Suggestion:** Omit this constructor in the target implementation. If stack-trace suppression is needed for performance, use the target language's own idiom, such as lightweight error values in Go.

### `User (@Entity schema auto-generation via Hibernate ddl-auto)` in `src/main/java/com/smartContact/model/User.java`
**Reason:** Hibernate derives and applies DDL from annotations at startup. Most target frameworks have no implicit equivalent, so the schema must be created by an explicit step.
**Suggestion:** Define the model in the target ORM with identical table/column names, types, lengths and unique constraint, and quote the reserved table name USER. Wire schema creation into app startup (create_all/sync) or into a migration that the project's start/run command actually executes.

### `Lombok annotations (@Data, @Builder, @NoArgsConstructor, @AllArgsConstructor)` in `src/main/java/com/smartContact/model/User.java`
**Reason:** This is compile-time code generation specific to Java.
**Suggestion:** Replace with native language constructs: dataclass/record/struct with explicit fields, a constructor or builder, and a toString/serializer that excludes the password.

### `@NotBlank Bean Validation` in `src/main/java/com/smartContact/model/User.java`
**Reason:** This is a JSR-380 annotation evaluated by Hibernate Validator through Spring's @Valid integration.
**Suggestion:** Use the target's validation library (e.g. a schema validator or request DTO validation) with a not-blank rule (trimmed length > 0) and the same error message.

### `UserDao (JpaRepository runtime proxy / query derivation)` in `src/main/java/com/smartContact/repository/UserDao.java`
**Reason:** Go has no runtime proxy generation or method-name-based query derivation. The implementation is synthesized by Spring Data at startup, so no source exists to translate.
**Suggestion:** Manual rewrite: hand-write a Go struct implementing an explicit UserRepository interface with sqlx (explicit SQL) or gorm (db.Where("name = ?", name).First(&u)). Implement only the CRUD methods actually used by callers.

### `JPA persistence context semantics (lazy loading, dirty checking, cascades) inherited via JpaRepository` in `src/main/java/com/smartContact/repository/UserDao.java`
**Reason:** These are Hibernate runtime behaviours with no equivalent in sqlx. gorm only partially emulates cascades and associations, and has no dirty checking or lazy proxies.
**Suggestion:** Make all loading, updating and cascading explicit in service-layer code within explicit transactions. Use DB-level foreign keys with ON DELETE CASCADE where appropriate.

## Observer agent findings

The Observer agent monitored the migration and identified these patterns:

- **After 3 modules:** Implicit Spring/Hibernate framework behavior (schema auto-creation and sequence-based id generation) is only partly translated and never wired into the Go application's startup, which leaves the User entity non-functional end to end.
- **After 6 modules:** Implicit Spring/Hibernate runtime behavior (auto schema creation, sequence-based id generation, bootstrap wiring) is not captured as specs or wired into main, so persistence modules come out structurally present but functionally dead.

## Files requiring manual review

These files were migrated but scored below the confidence threshold.
Review them carefully before merging.

### `src/main/java/com/smartContact/error/RestResponseEntityExceptionHandling.java`
Confidence: 84%
Issues:
  - [info] A Java null message would serialize as "message": null. In Go, an error message is always a string, so the body gets "" instead of null.

### `src/main/java/com/smartContact/service/UserServiceImp.java`
Confidence: 72%
