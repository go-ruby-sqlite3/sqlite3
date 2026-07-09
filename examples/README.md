# Examples

Runnable pure-Ruby usage of the `SQLite3::Database` / `SQLite3::Statement` API, verified under the [rbgo](https://github.com/go-embedded-ruby/ruby) interpreter.

```sh
rbgo examples/sqlite3_usage.rb
```

| File | Shows |
| --- | --- |
| `sqlite3_usage.rb` | Opening an in-memory database, `execute` with positional binds, `last_insert_row_id` / `changes`, `get_first_row` / `get_first_value`, `results_as_hash`, prepared statements (`prepare` / `bind_param` / `execute`), `transaction` commit and rollback, and rescuing a `SQLite3::Exception` on a SQL error |
