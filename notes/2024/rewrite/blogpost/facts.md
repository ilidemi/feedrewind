### What is being rewritten

A web app, web and worker roles. Needs to be ready to encounter an arbitrary blog and crawl it, at a rate of 1 request per second. Crawling job can take anywhere from 1 second to 20 minutes.

App is written in Rails, with Postgres as the only data storage, also used for queues.

### Why

Main reason is very silly resource consumption. If two users type in two new blogs, we need two long-running jobs. The popular library that runs Rails jobs on top of Postgres expects each worker to have its own process, starting at 150MB RAM per worker. Two processes need a third one to manage them, totaling 450MB RAM. Basic Heroku node comes with 512MB RAM - we shouldn't need more than that for parsing two HTMLs a second, right? It may look like it fits but this is only the starting memory usage - when crawling actually goes off, we immediately go over the limit and stay there, as Ruby garbage collector interacts with Heroku platform in a way where the memory quota warnings are spammed but the memory doesn't actually get released. The options are to experiment with alternative runtimes or less popular libraries, to live in a universe where we need one backend computer per user, or to use a tech stack that's efficient from the start.

Go is that efficient tech stack. I was very new to it but implementing a full app is a great opportunity to learn. Also, a rewrite scratches the programmer's itch to look what's under the hood - even if the sum total of Rails' batteries hasn't worked out for the product, each individual battery is battle-tested and can be learned from.

### The Plan

A big endeavor needs a plan - it would be unfortunate to get bogged down in some piece midway, not know where you are and how much is left and lose the motivation to finish. I decided to err on the thorough side and try to think from as many angles as possible.

* Did anyone write up a similar migration? - a few blogposts, all are very high level
* What is Rails doing for me at the frontend?
	* AJAX forms and POST links/buttons with rails-ujs
	* Asset pipeline
	* Tailwind CSS integration
	* View layouts
	* Session management
	* CSRF
	* Encrypted cookies
	* Auth
	* ActionCable (to report crawling progress)
	* Form tag helpers
* What is Rails doing for me at the backend?
	* Enable/disable auth per action
	* Default middleware
	* ActiveRecord created_at, updated_at
	* DB migrations
	* Configuration
	* Logging
* Is everything available in Go?
	* Which HTTP framework
	* DB access
	* Websockets
	* Nicer logging
	* Testing framework
	* E2E tests with Puppeteer
	* Debugging experience
* Does the jobs DB schema seem possible to pick up from Go code?
* Find replacements for every Ruby dependency
* Replace the workflows that were relying on Rails console
* Read through all the code, mentally map patterns to the new language
* How will runtime metrics be reported in production?
* Determine project structure
* Sequence the tasks
* How will I rollout?
* How will I validate that the new code works well?

### The Plan - Details

The frontend parts were carefully reimplemented by hand, except rails-ujs - it's convenient, small, easy to pull in, and deals with browser quirks I'd rather not deal myself. On the backend, DB migrations, config, logging, job infra and most of the middleware were reimplemented in a nicely vertically integrated way. DB created_at and updated_at moved to Postgres defaults and triggers. Rest of Rails conveniences were replaced by chi, gorilla/securecookie, gorilla/websocket, jackc/pgx, rs/zerolog and stretchr/testify.

Nontrivial dependencies?

Sequencing