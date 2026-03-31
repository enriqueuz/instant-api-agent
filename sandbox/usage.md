# Song API Usage Guide

This guide provides `curl` examples for interacting with the Song API.
Replace `$SERVER_URL` with the actual base URL of your running API (e.g., `http://localhost:8080`).

## 1. Get All Songs

Retrieve a list of all available songs.

    curl "$SERVER_URL/songs"

## 2. Get Song by ID

Retrieve a single song by its unique identifier (`alltime_rank`).

    curl "$SERVER_URL/songs/1"

## 3. Pagination

Control the number of results and navigate through pages using `limit` and `page` query parameters.

### Limit results

Retrieve the first 5 songs.

    curl "$SERVER_URL/songs?limit=5"

### Retrieve a specific page

Retrieve the 2nd page with 3 songs per page.

    curl "$SERVER_URL/songs?limit=3&page=2"

## 4. Sorting

Sort the results by any column in ascending (`asc`, default) or descending (`desc`) order using `sort` and `order` query parameters.

### Sort by 'song_title' (ascending)

    curl "$SERVER_URL/songs?sort=song_title&order=asc"

### Sort by 'total_streams_billions' (descending)

    curl "$SERVER_URL/songs?sort=total_streams_billions&order=desc"

### Sort by 'release_year' (descending)

    curl "$SERVER_URL/songs?sort=release_year&order=desc"

### Sort by 'alltime_rank' (ascending, default order)

    curl "$SERVER_URL/songs?sort=alltime_rank"

## 5. Filtering

Filter songs by exact matches on any column. Multiple filters can be combined.
Note: The current API implementation only supports exact match filtering. Range queries (e.g., `min_XX`, `max_XX`) are not supported.

### Filter by 'alltime_rank'

    curl "$SERVER_URL/songs?alltime_rank=1"

### Filter by 'song_title'

    curl "$SERVER_URL/songs?song_title=Blinding Lights"

### Filter by 'artist'

    curl "$SERVER_URL/songs?artist=The Weeknd"

### Filter by 'total_streams_billions'

    curl "$SERVER_URL/songs?total_streams_billions=4.00"

### Filter by 'primary_genre'

    curl "$SERVER_URL/songs?primary_genre=Pop"

### Filter by 'bpm'

    curl "$SERVER_URL/songs?bpm=171"

### Filter by 'release_year'

    curl "$SERVER_URL/songs?release_year=2019"

### Filter by 'artist_country'

    curl "$SERVER_URL/songs?artist_country=Canada"

### Filter by 'explicit'

    curl "$SERVER_URL/songs?explicit=false"

### Filter by 'danceability'

    curl "$SERVER_URL/songs?danceability=0.71"

### Filter by 'energy'

    curl "$SERVER_URL/songs?energy=0.73"

### Filter by 'valence'

    curl "$SERVER_URL/songs?valence=0.33"

### Filter by 'acousticness'

    curl "$SERVER_URL/songs?acousticness=0.00"

### Filter by 'dataset_part'

    curl "$SERVER_URL/songs?dataset_part=PartA"

## 6. Combined Queries

Combine filtering, sorting, and pagination parameters.

### Filter by genre and release year, sort by streams (desc), limit to 2 results on page 1

    curl "$SERVER_URL/songs?primary_genre=Pop&release_year=2019&sort=total_streams_billions&order=desc&limit=2&page=1"

### Filter by artist country and explicit content, sort by BPM (asc), limit to 3 results on page 2

    curl "$SERVER_URL/songs?artist_country=United Kingdom&explicit=false&sort=bpm&order=asc&limit=3&page=2"