1. Code Style and Philosophy
   - Write code following the principles from *Go Proverbs*.
   - In every file, leave comments in English explaining why things are done this way.
2. Code Structure
   - Group functions and structs logically within each file.
   - Separate code blocks with comments so related functionality is easy to find.
   - For readability, split the entire project into separate logical packages.
3. Concurrency
   - Use *goroutines* and channels, as well as `select/case`.
   - Do not use `mutex`; handle synchronization only through channels.
   - Break large tasks into smaller ones and connect them via channels.
4. Libraries and Dependencies
   - Use only Go’s standard libraries.
   - Anything non-standard should be added manually, taking inspiration from other projects.
5. Database Work
   - Use the standard `database/sql` package.
   - Write SQL queries to be as universal as possible, avoiding database-specific constructs.
   - Any DB-specific logic should be handled in Go code instead.
6. Performance
   - Optimize to avoid blocking operations.
   - Process data asynchronously, in a streaming fashion.
7. Code Response Format
   - Always show the entire function.
   - When modifying part of the code, show the snippet with both the start and end, inserting the new part in between.
   - Do not use diff format.
8. Code Comments
   - Write all code comments in English.
