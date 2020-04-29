# Roblox Lua API Archive
This repository contains an archive of versions of the Roblox Lua API.

## Structure
Files in this repository can be accessed through HTTP via the following URL:

	https://raw.githubusercontent.com/RobloxAPI/build-archive/master/

The `data` directory contains builds grouped by certain properties of the
content. Additionally, the `groups.json` file contains an array of strings,
where each string indicates the name of a group.

A group directory contains the following files:

- `builds`: A directory containing build files.
- `metadata.json`: Contains metadata about each build.
- `latest.json`: Contains metadata about the latest build.

Within the `builds` directory, each subdirectory is named according to the
Version GUID of the build. Contained within each build directory are the files
of the build. The exact files included depend on the group.

Summary:

	data/groups.json
	data/<group>/builds/<version-guid>/<file>
	data/<group>/metadata.json
	data/<group>/latest.json

Example:

	data/groups.json
	data/legacy/builds/version-6b060bf0723c4a04/API-Dump.json
	data/legacy/metadata.json
	data/legacy/latest.json

### Metadata
The `metadata.json` file contains an object with the following fields:

Field     | Type                    | Description
----------|-------------------------|------------
`Files`   | array of string         | List of files expected in each build.
`Builds`  | array of Build          | List of builds present in the group, ordered by Date.
`Missing` | MissingFiles (optional) | Indicates files that are missing from the archive.

**Build** is an object with the following fields:

Field     | Type   | Description
----------|--------|------------
`GUID`    | string | The version GUID of the build. Corresponds to the name of a directory under `builds`.
`Date`    | string | Roughly when the build was produced, formatted according to RFC3339.
`Version` | string | The version of the build, formatted as dot-separated components (e.g. `0.123.1.12345`).

Note that multiple Builds within the array can have the same GUID. They will
have differing dates in this case, indicating that the same build was produced
multiple times.

**MissingFiles** is an object where each field name is a GUID corresponding to a
build, and the value is an array of strings. Each string indicates the name of a
file that is missing from the build.

Example:

```json
{
	"Files": [
		"API-Dump.json",
		"API-Dump.txt",
		"ReflectionMetadata.xml"
	],
	"Builds": [
		{
			"GUID": "version-87de5333d4254860",
			"Date": "2011-10-25T23:33:20-07:00",
			"Version": "0.47.0.380"
		}
	],
	"Missing": {
		"version-87de5333d4254860": [
			"ReflectionMetadata.xml"
		]
	}
```

The `latest.json` file contains a single Build object, indicating the latest
produced build. This is updated if the latest build changes.

### Groups
This repository has the following build groups:

#### Legacy
The `legacy` directory contains builds that did not originally have API dumps in
JSON format. Each build contains the following files:

- `API-Dump.txt`: API dump in the original format.
- `API-Dump.json`: Translated from the original dump format. Content may change
  over time as accuracy is improved.
- `ReflectionMetadata.xml`: Reflection metadata file.
