# chastikey-alexa

This is a simple skill that anyone can deploy to a server and then use
Alexa "developer" skills to add it to their personal device.

Then you can say something like 

> Alexa ask chastikey for status

and it will reply with a list of your locks, and how long they've run
for.

Setup is a little complicated, because we're playing with developer mode.

## Prerequisites:

A server with a DNS name and a TLS certificate.  This code does not
do TLS itself, so a TLS proxy webserver is needed (I use Apache; see
below).

An account with the [Chastikey app](https://chastikey.com/)

An API key and secret, generated from the Chastikey app, required to
talk to [v0.5 of the API](http://api.chastikey.com/docs/).

## GoLang dependencies:

I use the following modules (`go get` them)

    github.com/mikeflynn/go-alexa/skillserver
    github.com/tkanos/gonfig

These have other dependencies that may need to be satisfied.  At the
time of writing that includes

    github.com/gorilla/mux
    github.com/urfave/negroni

## Build the code

    go build -o chastikey *.go

Or use the `Makefile`

## Testing the API configuration

You need to build a config file in `$HOME/.chastikey`

If the file doesn't exist then you'll get an error such as:
 
    % ./chastikey 
    Using configuration file /home/bdsm/.chastikey
    Skill ID is not defined.  Aborted

On Windows it will look for `%HOMEDIR%%HOMEPATH%` or `%USERPROFILE`.
On Linux/MacOS it will look in `$HOME`.  Examples may be:

    C:\Documents and Settings\bdsm\.chastikey
    C:\Users\bdsm\.chastikey
    /Users/bdsm/.chastikey
    /home/bdsm/.chastikey

It needs four values:

    SkillID   -- this is the skill ID from the Alexa Console (see later)
    ApiID     -- this is your Chastikey API Key generated from the App
    ApiSecret -- this is your Chastikey API Secret generated from the App
    Username  -- this is your DiscordID from the Chastikey Discord server

You can join the Discord server from the menu at [Chastikey app](https://chastikey.com/).  You can find your DiscordID from the [support docs](https://support.discordapp.com/hc/en-us/articles/206346498-Where-can-I-find-my-User-Server-Message-ID-)

eg

    % cat ~/.chastikey
    {
      "SkillID":"amzn1.ask.skill.12345678-9abc-def0-1234-56789abcdef0",
      "ApiID":"1234567890abcdefghijklmnopqrstuv",
      "ApiSecret":"1234567890abcdefghijklmnopqrstuv",
      "UserName":"yourdiscordid"
    }


Can test the code talks to the API from the CLI

    % ./chastikey status
    Using configuration file /home/bdsm/.chastikey
    Calling listlocks.php
    You have 1 lock.  Lock 1 is held by keyholder, and has been running for 81 days 10 hours 24 minutes 20 seconds.  

# Setting up the reverse proxy

This code does NOT do https itself; it's http on port 3001.
So you need have a HTTPS reverse proxy (eg nginx) in front of it.

It has a handler for `/echo/chastikey`.  When the server is running
you can test with something like

    % curl localhost:3001
    404 page not found

    % curl localhost:3001/echo/chastikey
    Not Authorized

For the reverse proxy I use Apache with TLS and Proxy rules, such as

    ProxyPass        /alexa-chastikey/  http://myserver:3001/echo/ retry=0
    ProxyPassReverse /alexa-chastikey/  http://myserver:3001/echo/

Note the `echo/` part means we don't need to add that to the Amazon
config later

That allows me to have 

    https://my.server.example/alexa-chastikey/chastikey

as the Amazon endpoint Amazon will use.   

(If you follow the rewrite this becomes `myserver:3001/echo/chastikey` and
so you can see the `/echo/chastikey` requirement is met)

I do it this way because I have a single webserver with multiple rules
that can handle lots of different skills.

## Setting up the Amazon side.

This is the hardest part of the setup, and something that you'll
probably need to do a dozen times before it works!  I can't really
help here.

You need to do this from the Amazon Developer portal

    https://developer.amazon.com/alexa/console/ask

Build a new application.

The file `README.json` provides a lot of stuff for the build/intent model.
We only have one command "status" so it's pretty simple.

Ensure the endpont is https (not Lambda)

URL needs to be https; eg `https://my.server.example/alexa-chastikey/chastikey`

Make sure the SSL cert stuff is defined properly.  I use a wildcard cert
from LetsEncrypt so I pick "subdomain of wildcard".


From the developer dashboard you can find the Skill ID.  This is the
value that you need to put into the `.chastikey` configuration file.

Once the config file is updated you can run the application; it will start
displaying log messages

    % ./chastikey
    Using configuration file /home/bdsm/.chastikey
    [negroni] listening on :3001

## Debugging

One of the main problems you'll get is the error messages are generic.
If you get any of the stuff wrong anywhere then you'll normally get a
very unhelpful "There was a problem with the requested skills response".

But if you see log entries appear in this file then you've probably
got the communication path set properly!

* So try to connect to the port 3001 server.  That will verify the server
is running

* Try to connect to the https URL defined in the Alexa skill.  That will
verify TLS is working and forwarding traffic properly.

Everything else is likely in the developer console!

## License

This program is free software; you can redistribute it and/or
modify it under the terms of the GNU General Public License
as published by the Free Software Foundation; either version 2
of the License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program; if not, write to the Free Software
Foundation, Inc., 51 Franklin Street, Fifth Floor, Boston, MA  02110-1301, USA.
