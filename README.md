# Wormhole

This tries to make a self-hostable load-balancer. Such a service is meant to replace SaaS software that does something similar. The software could cut out nginx and allow load balancing by simple adding an executable and environment variable to deployed Docker swarms across multiple regions.

> ⭐️ This repo lives at [Wormhole on Gitlab](https://gitlab.com/NoahGray/wormhole)

Furthermore, this load-balancer may be programmable into a CDN, or other "edge" web service. This is all a matter of learning how this technology can be most easily and securely deployed on off-the-shelf VPS services like [Linode](https://www.linode.com/?r=648a8ae1ae75b0fe6d5753224c20e453a47badaf) (referral links herein). Linode's VPS cloud products are comparable (virtually identical) to [Digital Ocean](https://m.do.co/c/fa21c7c32e3f) droplets in specs and price, take your pick.

## Goal

For now, I hope to allow folks to simply deploy load balancing, caching and some other features without subscribing to a fixed-cost SaaS that charges per app or per hour.

We might also iamgine a simple service like Dokku (a self-hostable Heroku-like toolset) combined with these features either as a plugin or a fork.

It would be nice to have 4-5 inexpensive VPS around the world to easily deploy to, with a Heroku-esque workflow, some JS, and caching. That could create a very modern tool for folks who think about the world beyond the West.

# License and contributors

Thanks to [Superfly](https://github.com/superfly) for getting things started.
