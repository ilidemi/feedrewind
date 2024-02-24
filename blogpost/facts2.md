### What is being rewritten

A web app, web and worker roles. Needs to be ready to encounter an arbitrary blog and crawl it, at a rate of 1 request per second. A crawling job can take anywhere from 1 second to tens of minutes.

App is written in Rails, with Postgres as the only data storage, also used for queues.


### Why rewrite?

Main reason is very silly resource consumption. If two users type in two new blogs, we need two long-running jobs. The popular library that runs Rails jobs on top of Postgres expects each worker to have its own process, starting at 150MB RAM per worker. Two processes need a third one to manage them, totaling 450MB RAM. Basic Heroku node comes with 512MB RAM - we shouldn't need more than that for parsing two HTMLs a second, right? It may look like it fits but this is only the starting memory usage - when crawling actually goes off, we immediately go over the limit and stay there, as Ruby garbage collector interacts with Heroku platform in a way where the memory quota warnings are spammed but the memory doesn't actually get released. The options are to experiment with alternative runtimes or less popular libraries, to live in a universe where we need one backend computer per user, or to use a tech stack that's efficient from the start.

Go is that efficient tech stack. I was very new to it but implementing a full app is a great opportunity to learn. Also, a rewrite scratches the programmer's itch to look what's under the hood - even if the sum total of Rails' batteries hasn't worked out for the product, each individual battery is battle-tested and can be learned from.


### How it was planned

Some people may be able to just start writing code but I'm not one of those people. I wanted to thoroughly avoid the situations where a big unknown pops up and destroys the motivation, or several unknowns come in a quick succession and compound on each other. A sequential plan made of a lot of small checkboxes makes it so that you always know where you are and the scale of any possible problem is limited.

I planned this in three passes:
1. Think of all the angles to look at the codebase from
2. Fill in the details
3. Make a sequence of checkboxes where every detail is included somewhere

Here are the angles I could think of when making the plan:
* Did anyone write up a similar migration?
	* A few blogposts, all are very high level
* What is Rails doing for me at the frontend?
	* Asset pipeline, view layouts, session management, CSRF, bcrypt invocation will be carefully reimplemented by hand
	* AJAX forms and POST links/buttons are supplied by rails-ujs which is okay to keep - it's convenient, small, easy to pull in, and deals with browser quirks I'd rather not deal myself
	* Webpack will be replaced with just pulling in the two frontend dependencies as templates
	* ActionCable will be pulled into one page that's actually using it (crawling progress indicators), reducing the amount of code by a lot
	* Form tag helpers are sad to miss but raw HTML is not that much boilerplate
	* Tailwind CSS will be integrated as checked in binaries that are invoked by a separate command
* What is Rails doing for me at the backend?
	* DB migrations, configuration, logging, most of the middlewares are to be reimplemented in a nice vertically integrated way
	* ActiveRecord queries will be replaced with raw SQL, roughly each "model" having a file and each operation having a function in it (note from the future: this is a safe starting point, but ok to relax at the end)
	* ActiveRecord created_at and updated_at will move to Postgres defaults and triggers
* What replacement facilities are available in the Go ecosystem?
	* Chi, gorilla/securecookie, gorilla/websocket, jackc/pgx, rs/zerolog and stretchr/testify look good
	* Debugging experience with VSCode is not bad - I decided to move off RubyMine at the same time
* Will I need to migrate the jobs database?
	* The db schema from the stack of libraries is Ruby objects serialized as YAML and put into one text column. Annoying to deal with but possible and enables flipping between codebases if needed
* Find replacements for every Ruby dependency
	* All dependencies have a direct equivalent (note from the future: this was not the case)
* Replace the workflows that were relying on Rails console
	* The actions were pretty much just reading and modifying data, instead I can directly attach DBeaver and ensure the `ON DELETE CASCADE` constraint where necessary
* Read through all the code, mentally map its patterns to Go
	* Exceptions straightforwardly convert to panics - the stack trace is preserved, Recoverer middleware seems to be standard practice, and most of exceptions in this codebase are going to the top anyway (note from the future: **rookie mistake, do not do this**)
	* Somewhat dynamic types should become static types - extra work, but I sometimes can't tell which fields are supposed to be set at which point during multi-stage parsing
	* `ensure` becomes `defer func(){...}()`
	* Moving functions around and merging files is okay
	* ActiveRecord exceptions have pgx error equivalents
	* AcitveRecord validations move to the database constraints or a common function
	* In Ruby I could put `@is_user_logged_in` and `@has_email_bounced` in the controller base and use them from every view, in Go all templates will need a one stop shop that ensures these fields are present in some common way
	* Time travel infra for E2E schedule tests will become a util enforced by a linter rather than monkey-patching `DateTime.now` (note from the future: just a util, a careless `time.Now` call would fail a test anyway)
	* The test suite for crawling blogs moves from processes to goroutines
	* Fuzzy date parsing will need a unit test dataset comparing the results with Go implementation
* How will runtime metrics be reported in production?
	* Heroku got me covered
* Determine project structure
	* Ended up being part Standard Go Project Layout, part how it was in Rails, and part what made sense for the project.
* How will I validate that the new code works well?
	* Review of the test coverage showed that the tests are actually quite thorough. Yay past me who wanted to do a good job
* How will I roll out?
	* Shut down the old codebase, back up the DB, start up the new codebase. Being able to afford hours of downtime is a luxury but why not use it when you can
* Sequence the tasks
	* My mentor suggested to implement a small component fully, then expand depth first. It should start to get easier and easier to convert things as the execution goes on

[Toggle details]

For the final sequence, I decided to start small, make every piece completely solid before moving onto the next task, and expand depth first. Common infrastructure is to be added in chunks as needed, which is more motivating than trying to perfect the foundation with nothing on the screen.

Final sequence:
* Set up the dev environment
	* Folders for the project structure are created here
* Port the landing page
	* Logging, common middleware, asset pipeline, and Tailwind CSS integration come in here
* Port the web role, one route at a time
	* Login page comes with configuration, DB migrations, DB access infra, encrypted cookies middleware, session management, CSRF and the one stop shop for templates
	* Sign up page comes with bcrypt invocation
	* Rest of the pages pull in the functionality as needed
	* Run E2E tests with Go web role and Rails worker
* Port the crawler
	* Standalone test infra is set up first. Now we have a number that can go up
	* Blog patterns come in from simple to complex, watching the number go up
* Port the worker
	* Job infra comes first
	* Jobs come in from simple to complex
* Prepare the production environment
	* Set up Procfile and metrics, ensure config variables
* Rollout and validate

### How did it end up / Lessons learned

The rewrite succeeded and took 267 hours in total. Time spent on planning absolutely paid off - 11 hours it took seemed like a lot of effort in the beginning but it resulted in very manageable slogs and was dwarfed by 249 hours of implementation time.

Here's the total breakdown:
Planning 11:19
	Upfront 9:33
	Before crawler 1:19
	Before deployment 0:27
Coding 248:57
	Infra 50:58
	Date parse (explained below) 12:38
	App 185:21
Deployment 7:03

Good test suite was a must. Even though I authored the original code and was clicking through every piece implemented, tests caught many typos and it turns out the rewriting mind doesn't like to think much about the corner cases. The most helpful tests were the ones around parsing and feed generation that have a lot of well-defined inputs and outputs, as well as end-to-end tests that go from sign up to the blog post showing up in the mail or RSS feed.

A strict boundary between working and in progress code was a must. I made a habit of always leaving a `// TODO` comment if the current function is missing any functionality and I have to switch out of it. Some days you'll be scatterbrained and need those breadcrumbs. Sometimes it helps to skip three lines to avoid architecting some other subsystem right now, but you need to remember to come back later.

Not all dependencies had a perfect replacement. A minor case, data contracts in postmark-go were missing a field that my testing relied on, so the library had to be vendored. A bigger one was fuzzy date parsing - the blog posts can be listed out of order, the dates for them can be somewhere on the page, the text of the date can be surrounded by extra words, and we have to be able to unscramble it all. The popular library in Go expects the string to be just the date, while Ruby's `Date._parse` can extract it from anywhere in the string, so I had to take extra 12+ hours to port the latter from C to Go. Thankfully, it was well-structured and well-endowed with tests.

Some infrastructure decisions needed to be changed halfway through. I think it to be a part of learning, and the thorough plan being explicitly there to enable it. With a sequence of granular tasks, you get many opportunities to notice how are things working out and go back for a round of sweeping changes when needed, not having to worry about multiple things being "in progress". The alternative is to assume that the upfront design has to be perfect, which introduces pressure and doesn't do good for the long-term motivation.

Let's go through some examples of the infrastructure needing a change.

For error handling, at first it seemed that panics are a straightforward replacement for exceptions - the call stack is automatically included, and the handling almost exclusively happens at the top level. Then a few cases came up where multiple paths want to query the same DB row, some expect it to be found, and some want to explicitly do something different when it's not found. Introducing `recover` would make it even further from the Go way. Using errors when a row is not found and panics on a network error would be messy. But the raw errors are underpowered - they come without stack traces, with the Go book suggesting to manually add more context into the message at every level. The solution was to introduce a special type of error that captures the stack on creation, similar to `go-errors/errors`, and make sure to use it everywhere in the codebase. It helped to have a first party wrapper around the DB library that only returns those, as 95% of error handling in the app is around DB calls. Rookie mistake, but the thorough planning allowed me to go back and fix it without losing confidence in the rest of the project.

For logging, I knew that it needs to be a common layer that logs the request id and the user id by default, with as little typing at call sites as possible. But request ids are only a thing in the web role, and I explicitly left worker design for later on, while the logging infra would be needed from the start. Making the layer work for just the web role then refactoring once the worker is ready worked out just fine, and was a matter of a lighthearted find and replace pass.

For DB calls, I initially decided to be conservative and have a function in the "model" file for every query. Then a lot of single-use functions with idiosyncratic return types started popping up. Somewhere around `type SubscriptionUserIdStatusScheduleVersionBlogBestUrl struct` I started questioning this approach. It was still worth it to handle insertion invariants and dependent records in one place, and some read queries were getting reused, so I did a sweep of only inlining the idiosyncratic read calls. Inlining SQL queries is way easier than noticing when to uninline, so everything came together well: conservative approach was a good default, thorough planning allowed to cleanly come back at the appropriate time, and the decision came out of looking at the real code.

Worker was the one piece of infrastructure that's not a 1:1 equivalent. 