# lilgraph

A Little Graph Language -- a plain text format for describing simple directed graphs.

The format is inspired by [DOT](https://graphviz.org/doc/info/lang.html), with a few bits stripped out or tweaked, and a hint of [Cypher](https://en.wikipedia.org/wiki/Cypher_(query_language)) thrown in.

## Example

```
// A node is an id, with optional attributes in square-brackets after it. All
// attrs are treated as strings.
// The first attr item can be standalone; this is treated specially, as the
// Type property, and isn't part of the attrs map.

luke [human; homeplanet=tatooine]
leia [human; homeplanet=alderaan]

// Types are optional, though:

chewbacca [homeplanet=kashyyyk]

// Nodes don't _need_ to have attributes, though. A bare id is a valid node:

obi_wan

// Duplicate nodes are merged. Later attributes win. So these two:

luke[saber_color=blue force_sensitivity=high]
luke[saber_color=green]

// ... are equivalent to:

luke[saber_color=green force_sensitivity=high]

// A specific node id can only have one type value declared. That value can be
// repeated, or omitted, later; but can't change. So these are OK...

luke [human]
luke

// ...but this would fail:

# luke[forceghost]

// Edges can be declared between any node ids.

obi_wan -> luke

// To form edges, nodes don't have to be declared beforehand.

r2d2 -> sandcrawler
c3po -> sandcrawler

// Edges can have attributes too. These are declared in the middle of edges.
// Like nodes, the first value can be standalone and is treated as "type".

luke -[member]-> jedi
leia -[member]-> rebel_alliance

// If you need big edge attribute lists, you can split them over multiple lines.

luke -[member;
    callsign="Red Five"
    primary_craft="X-Wing"
    pilot_training_qualification="Bullseye'ing womprats in his T16"
]-> red_squadron

// Edges can be chained.
yoda -[trained]-> dooku -[trained]-> qui_gon -[trained]-> obi_wan -[trained]-> anakin

// Like nodes, duplicate edges are merged. This is based on edge source, target
// and type.  So, these form three separate edges, one of them type-less:

obi_wan -> luke
obi_wan -[protected]-> luke
obi_wan -[trained]-> luke
obi_wan -[protected since=birth]-> luke
obi_wan -[trained when="0 BBY"]-> luke

// If you like alignment, edge arrows can be extended.

han_solo ---[owns]---> millenium_falcon
chewbacca -[crew_of]-> millenium_falcon

/*
C-style block comments are supported.
*/
# Hash comments are also supported
```
