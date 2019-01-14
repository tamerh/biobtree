export default class Fetch {
    constructor() {
        this.endpoint = "http://localhost:8888/ws/";
        //this.endpoint="http://xxx:8080/ws/api?ids=";
        //this.endpoint = "https://www.ebi.ac.uk/~tgur/xrefmap/api.php?ids=";
        //this.xref_conf = XrefConf;
    }

    /**
     * Search id and parse with colfer.
     */
    search(id, callback, callback_params) {
        // id.replace(/ /g, '') white space clear
        fetch(this.endpoint + "?ids=" + id)
            .then(res => res.json())
            .then((out) => {
                callback(out, callback_params);
            })
            .catch(err => {
                throw err
            });
    }

    searchByFilter(sub_result, callback, callback_params) {

        let url = this.endpoint + "filter/?ids=" + sub_result.identifier + '&src=' + sub_result.domain_id + '&filters=' + sub_result.filters;

        if (sub_result.lastFilteredPageKey && sub_result.lastFilteredPageKey.length > 0) {
            url += "&last_filtered_page=" + sub_result.lastFilteredPageKey;
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

    /**
     * Search id and parse with colfer.
     */
    searchByPageIndex(id, source_domain, page, page_total, callback, callback_params) {
        // id.replace(/ /g, '') white space clear
        fetch(this.endpoint + "page/?ids=" + id + '&src=' + source_domain + '&page=' + page + '&total=' + page_total)
            .then(res => res.json())
            .then((out) => {
                callback(out, callback_params);
            })
            .catch(err => {
                throw err
            });
    }



}