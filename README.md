# Nextpipe

> This is a work in progress in the early stages. Currently the code is practically the same as [patchwork](https://github.com/tionis/patchwork)

Nextpipe is a simple communication backend to distribute data mainly meant for scripts and small applications.  
It's design is based [patchbay.pub's](https://patchbay.pub) with the addition of an authentication layer based
on tokens fetch from a nextcloud instance (or any Webdav host).  
I previously built a similar solution using ssh-key signed tokens as auth strategy under the name [patchwork](https://github.com/tionis/patchwork).  
This service can then be used to power use cases like static file hosting, file sharing,
cross-platform notifications, webhooks handling (including the maybe simplest CI setup using
traditional git forges), smart home routing, IoT Reporting, job queues, chat systems, bots....
All without bothering with writing a proper server hosting setup. For many use case curl and bash are enough.

## Usage

Nextpipe provides a nearly unlimited amount of virtual channels represented by a path and a namespace.
Data POSTed to a channel can be received by clients doing GET requests, the exact behaviour depends on the
type (specified with the `type` query parameter).
The available types are:

- **fifo/queue**: Each message is received by exactly one receiver, if
  no listeners are active the server blocks until there is one. This
  is the default mode.
- **pubsub**: All receivers receive the published message, if no
  listeners are active the server returns \"HTTP 204 NO CONTENT\".
- **blockpub/blocksub**: Same behaviour as pubsub, but the server
  blocks until there is at least one listener.
- **req/req**: Each request is matched one-on-one to a recipient who can also send data back.

### Query Parameters

- `type` -> set the type of query
- `persist=true` -> for listeners, don't close the request after receiving the first message/request
- `mime` -> set the mime-type at the other end of the channel
