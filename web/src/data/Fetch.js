export default class Fetch {
    constructor(endp) {
        this.endpoint = endp;

        this.endpointSearch = endp;
        this.endpointEntry = endp + "entry/";
        this.endpointMapfilter = endp + "map/";
        this.endpointFilter = endp + "filter/";
        this.endpointPage = endp + "page/";

        // this.endpointSearch = "https://www.ebi.ac.uk/~tgur/biobtree/search.php";
        // this.endpointEntry = "https://www.ebi.ac.uk/~tgur/biobtree/entry.php";
        // this.endpointMapfilter = "https://www.ebi.ac.uk/~tgur/biobtree/mapfilter.php";
        // this.endpointFilter = "https://www.ebi.ac.uk/~tgur/biobtree/searchfilter.php";
        // this.endpointPage = "https://www.ebi.ac.uk/~tgur/biobtree/searchpage.php";
    }

    search(id, page, filter, source, callback, callback_params) {
        // id.replace(/ /g, '') white space clear
        let url = this.endpointSearch + "?u=y&d=y&i=" + encodeURIComponent(id.trim());

        if (page.length > 0) {
            url = url + "&p=" + page;
        }
        if (filter.length > 0) {
            url = url + "&f=" + filter;
        }
        if (source.length > 0) {
            url = url + "&s=" + source;
        }
        callback_params[1] = url;
        fetch(url)
            .then(res => res.json())
            .then((out) => {
                callback(out, callback_params);
            })
            .catch(err => {
                var qError = {
                    "Err": err.message
                };
                callback(qError, callback_params);
            });
    }

    searchEntry(id, domain_id, callback, callback_params) {
        fetch(this.endpointEntry + "?i=" + encodeURIComponent(id.trim()) + "&s=" + domain_id)
            .then(res => res.json())
            .then((out) => {
                callback(out, callback_params);
            })
            .catch(err => {
                throw err
            });
    }

    mapFilter(id, mapfilter, page, source, callback, callback_params) {

        var url = this.endpointMapfilter + "?i=" + encodeURIComponent(id.trim());
        url = url + "&m=" + encodeURIComponent(mapfilter).replace(/%A0/g, "%20").replace(/%C2/g, "%20"); // workaround related with contenteditable

        if (page.length > 0) {
            url = url + "&p=" + page
        }

        if (source.length > 0) {
            url = url + "&s=" + source;
        }

        callback_params[1] = url
        fetch(url)
            .then(res => res.json())
            .then((out) => {
                callback(out, callback_params);
            })
            .catch(err => {
                var qError = {
                    "Err": err.message
                };
                callback(qError, callback_params);
            });
    }

    searchByFilter(sub_result, domain_id, callback, callback_params) {

        let url = this.endpointFilter + "?i=" + sub_result.identifier + '&s=' + domain_id + '&f=' + sub_result.filters;

        if (sub_result.lastFilteredPageKey && sub_result.lastFilteredPageKey.length > 0) {
            url += "&p=" + sub_result.lastFilteredPageKey;
        }

        fetch(url)
            .then(res => res.json())
            .then((out) => {
                callback(out, sub_result, callback_params);

            })
            .catch(err => {
                throw err
            });

    }

    searchByPageIndex(id, source_domain, page, page_total, callback, callback_params) {

        fetch(this.endpointPage + "?i=" + id + '&s=' + source_domain + '&p=' + page + '&t=' + page_total)
            .then(res => res.json())
            .then((out) => {
                callback(out, callback_params);
            })
            .catch(err => {
                throw err
            });
    }

}