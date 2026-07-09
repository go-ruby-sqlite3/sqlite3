# frozen_string_literal: true

require "sqlite3"

# Open a private in-memory database (a file path works the same way).
db = SQLite3::Database.new(":memory:")
db.execute("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, age INTEGER)")

# Insert with positional (?) binds, then read the generated rowid and row count.
db.execute("INSERT INTO users (name, age) VALUES (?, ?)", ["Ada", 36])
db.execute("INSERT INTO users (name, age) VALUES (?, ?)", ["Alan", 41])
puts db.last_insert_row_id # => 2
puts db.changes            # => 1

# A plain SELECT returns an Array of row Arrays.
puts db.execute("SELECT name, age FROM users ORDER BY id").inspect # => [["Ada", 36], ["Alan", 41]]

# Convenience readers: the first row, or a single scalar.
puts db.get_first_row("SELECT name FROM users WHERE age > ?", [40]).inspect # => ["Alan"]
puts db.get_first_value("SELECT COUNT(*) FROM users")                       # => 2

# results_as_hash yields column-keyed Hashes instead of Arrays.
db.results_as_hash = true
puts db.execute("SELECT name, age FROM users WHERE name = ?", ["Ada"]).inspect # => [{"name" => "Ada", "age" => 36}]
db.results_as_hash = false

# A prepared statement is compiled once and re-run with fresh binds.
stmt = db.prepare("SELECT name FROM users WHERE age > ?")
stmt.bind_param(1, 35)
puts stmt.execute.inspect # => [["Ada"], ["Alan"]]
stmt.close

# transaction commits on success and rolls back if the block raises.
db.transaction { db.execute("INSERT INTO users (name, age) VALUES (?, ?)", ["Grace", 85]) }
begin
  db.transaction do
    db.execute("INSERT INTO users (name, age) VALUES (?, ?)", ["Temp", 1])
    raise "abort this transaction"
  end
rescue RuntimeError
  # the "Temp" insert was rolled back
end
puts db.get_first_value("SELECT COUNT(*) FROM users") # => 3

# SQL errors raise a gem-faithful SQLite3::Exception subclass.
begin
  db.execute("SELECT * FROM missing_table")
rescue SQLite3::Exception => e
  puts e.class # => SQLite3::SQLException
end

db.close
puts db.closed? # => true
