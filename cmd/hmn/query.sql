WITH limited_urls AS (
    SELECT
        ml.link_id,
        ml.feed_root_canonical_url,
        rl.author,
        rl.channel,
        rl.url,
        ROW_NUMBER() OVER (
            PARTITION BY ml.feed_root_canonical_url, rl.url
            ORDER BY RANDOM()
        ) AS rn_url
    FROM matched_links ml
    JOIN raw_links rl ON ml.link_id = rl.rowid
    where (rl.url not like '%bvisness.me/apps%' and (rl.url not like '%bvisness.me/%' or author != 'bvisness')) and
    	(rl.url not like '%rfleury.com/%' or author != 'ryanfleury') and
    	author != 'abnercoimbre'
),
selected_urls AS (
    SELECT *
    FROM limited_urls
    WHERE rn_url <= 3
),
limited_authors AS (
    SELECT
        su.link_id,
        su.feed_root_canonical_url,
        su.author,
        su.channel,
        su.url,
        ROW_NUMBER() OVER (
            PARTITION BY su.feed_root_canonical_url, su.author
            ORDER BY su.link_id
        ) AS rn_author
    FROM selected_urls su
),
selected_links AS (
    SELECT *
    FROM limited_authors
    WHERE rn_author <= 3
),
mlinks AS (
	SELECT
	    sl.feed_root_canonical_url,
	    COUNT(*) AS total_matches,
	    -- Top Author
	    (
	        SELECT sl2.author || ' (' || COUNT(*) || ')'
	        FROM selected_links sl2
	        WHERE sl2.feed_root_canonical_url = sl.feed_root_canonical_url
	        GROUP BY sl2.author
	        ORDER BY COUNT(*) DESC
	        LIMIT 1
	    ) AS top_author,
	    -- Top Channel
	    (
	        SELECT sl3.channel || ' (' || COUNT(*) || ')'
	        FROM selected_links sl3
	        WHERE sl3.feed_root_canonical_url = sl.feed_root_canonical_url
	        GROUP BY sl3.channel
	        ORDER BY COUNT(*) DESC
	        LIMIT 1
	    ) AS top_channel,
	    -- Library Count
	    (
	        SELECT COUNT(*)
	        FROM selected_links sl4
	        WHERE sl4.feed_root_canonical_url = sl.feed_root_canonical_url
	          AND sl4.channel = 'the-library'
	    ) AS library_count
	FROM selected_links sl
	GROUP BY sl.feed_root_canonical_url
	ORDER BY total_matches DESC
)
SELECT * from mlinks
WHERE (total_matches >= 6 or library_count >= 3) and
	feed_root_canonical_url not in (
		'khronos.org/', 'gafferongames.com', 'cs.cmu.edu/', 'odin-lang.org', 'gnu.org/', 'theverge.com/',
		'jcgt.org/', 'lwn.net/', 'microsoft.com/en-us/research/', 'old.reddit.com/', 'unicode.org/',
		'joelonsoftware.com/', 'code.visualstudio.com/', 'dev.to/', 'gpuopen.com/', 'phoronix.com/', 
		'developer.chrome.com/', 'gamedev.stackexchange.com/', 'github.blog/', 'gravitymoth.com/', 
		'devblogs.microsoft.com/directx/', 'thephd.dev/', 'visualstudio.microsoft.com/', 'caniuse.com',
		'copetti.org/', 'git.musl-libc.org/cgit/musl/', 'theorangeduck.com/', 'aur.archlinux.org/', 
		'codegolf.stackexchange.com/', 'kickstarter.com/projects/', 'scattered-thoughts.net/', 
		'unix.stackexchange.com/', 'worrydream.com/', 'asawicki.info/', 'blog.cloudflare.com/',
		'blogs.windows.com/', 'css-tricks.com/', 'docs.rs/', 'wired.com/', 'web.dev/', 'warp.dev',
		'pcg-random.org/', 'wiki.libsdl.org/', 'unrealengine.com/', 'sublimetext.com/', 'sourceforge.net/',
		'pikuma.com/', 'devblogs.microsoft.com/visualstudio/', 'devblogs.microsoft.com/commandline/',
		'buttondown.email/hillelwayne/', 'bun.sh', 'andrewkelley.me/', 'techcrunch.com/', 'superuser.com/',
		'rudyfaile.com/', 'marctenbosch.com/', 'macrumors.com/', 'kotaku.com/', 'hacks.mozilla.org/',
		'gamedeveloper.com/', 'gamasutra.com/', 'eurogamer.net/', 'catch22.net/', 'bottosson.github.io/',
		'blog.selfshadow.com/', 'archlinux.org/packages/', 'tonsky.me/', 'theregister.com/',
		'steamdb.info/', 'solhsa.com/', 'love2d.org', 'hillelwayne.com/', 'go.dev/blog/', 
		'git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux.git/', 'ffmpeg.org/',
		'eli.thegreenplace.net/', 'dev.cancel.fm/', 'box2d.org/', '01.org/', 'snapnet.dev/', 
		'positech.co.uk/cliffsblog/', 'alain.xyz/', 'yosoygames.com.ar/', 'samwho.dev/',
		'kevroletin.github.io/', 'devblogs.microsoft.com/premier-developer/', 'cgg.mff.cuni.cz/',
		'ryanfleury.substack.com', 'devblogs.microsoft.com/cppblog/'
	)
