# Song API Usage Guide

The API provides access to the top 100 songs dataset. Replace `$SERVER_URL` with the address printed at startup.

### 1. Retrieve all songs
`curl $SERVER_URL/songs`

### 2. Retrieve a specific song by rank (ID)
`curl $SERVER_URL/songs/1`

### 3. Filter songs by artist
`curl "$SERVER_URL/songs?filter=artist:The Weeknd"`

### 4. Sort songs by title
`curl "$SERVER_URL/songs?sort=song_title"`