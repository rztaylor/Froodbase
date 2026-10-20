# Local development files

`users.example.json` is the sample static user list for the local stack. `make up` copies it to `users.json` (ignored by git) on first run; edit that copy to add users. Every sample user's password is `froodbase-dev`, and the hashes are for local development only. Generate a hash for a new user with:

```bash
echo -n 'new-password' | docker compose run --rm --no-deps -T api hash-password
```

See `docs/dev/ops/development.md` for the file format.

`postgres-init/` holds the scripts PostgreSQL runs once, when its data volume is first created: `10-app-role.sh` creates the service role `froodbase_app` with the password from `FROODBASE_APP_DB_PASSWORD` in `.env`. An existing volume from before this script needs `docker compose down -v` to pick it up.
