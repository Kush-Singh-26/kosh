# GO SSG

A static site generator (ssg) in golang.

- To build the site :

```bash
# Create a 'bin' directory to keep things organized 
mkdir bin 

# Compile the builder
go build -o bin/builder.exe ./builder/main.go

# Compile the server
go build -o bin/server.exe ./server/main.go
```

- To build the site

```bash
.\bin\builder.exe
```

- To start the server

```bash
.\bin\server.exe
```

## For continuous reloading :

**Terminal 1** :

```bash
air
```

**Terminal 2** :

```bash
.\bin\server.exe
```

---

- To create a new file

```bash
go run new.go "<title>"
```