# The human view: sources and schema

Used by /portfolio-run step 2 for each stock with no current `data/stocks/<T>/human_view.yaml`.

House rule 2: moat and fair value come from human analysts. In this order:
1. a research connector's sync, if one is installed;
2. a file the investor fills;
3. the cited web fallback: `./pb search "<company> economic moat fair value" --ticker <T>`, open the best hit with `./pb fetch-url <url> --ticker <T>`, and write the file from it with the date, `source` (the publisher), `source_url` and a `summary` of two or three sentences in your own words. Never copy the publisher's paragraphs, and never write a figure the page does not state;
4. none found: no file. The agents write "no human view" (DATA UNAVAILABLE).

The schema (`pb derive` and the agents read these keys; leave out what the source does not state):
```yaml
as_of: "<date read>"
source: <the research house or "investor">
source_url: <link, for a web source>
covered: true                 # a human analyst states the moat or fair value; false for a model-only rating
rating: <the house's own overall label, as stated>
moat: Wide | Narrow | None
moat_trend: Positive | Stable | Negative
fair_value: <number>
fair_value_currency: <ISO code; GBX, ZAc and ILA are read as cents>
fair_value_as_of: "<date>"
uncertainty: Low | Medium | High | Very High
capital_allocation: <the house's rating of management's capital allocation, as stated>   # the CEO filter reads it
bull_case: [<point>, …]        # in your own words
bear_case: [<point>, …]
summary: <two or three sentences in your own words>
```
A file with no `covered` key counts as covered when it states a moat or a fair value.
