web: feedrewind web --port $PORT
worker: feedrewind worker
release: sh -c 'feedrewind db migrate && feedrewind tailwind'
