export default class XrefModel {

    constructor(_fetcher, _xref_conf, _app_conf) {
        this.fetcher = _fetcher;
        this.xref_conf = _xref_conf;
        this.app_conf = _app_conf;
        this.app_comp = null;
        this.all_sub_results = [];
        this.result_counter = 0;
        this.hasGlobalFilter = this.app_conf.global_filter_datasets && this.app_conf.global_filter_datasets.length > 0;
    }

    setAppComp(_app_comp) {
        this.app_comp = _app_comp;
    }

    setGlobHasFilter(hasFilter) {
        this.hasGlobalFilter = hasFilter;
    }

    search(searchTerm, callback) {

        this.fetcher.search(searchTerm, this.processResults.bind(this));

    }

    processResults(data_results, callback) {
        this.all_sub_results = [];

        if (data_results == null) {
            this.app_comp.resultReady(-1);
            return;
        }

        if (data_results.length == 0) {
            this.app_comp.resultReady(0);
            return;
        }

        for (let key in data_results) {
            let results = data_results[key].results;

            for (let key2 in results) {
                this.prepareResult(results[key2], data_results[key], 0);
                if (!this.hasGlobalFilter) { // otherwise we should set after applying filter
                    this.addResult(results[key2]);
                }
            }
        }
    }

    addResult(result) {
        let ix = 0;
        while (ix < this.all_sub_results.length) {
            if (result.count > this.all_sub_results[ix].count) {
                break;
            }
            ix++;
        }
        this.all_sub_results.splice(ix, 0, result);
    }


    prepareResult(result, root_result, depth) {

        this.result_counter++;
        result.counter = this.result_counter;
        result.showResults = true;
        result.filterModalActive = false;
        result.treeModal = false;
        result.selectedXrefs = [];
        result.displayEntries = [];
        result.depth = depth;
        // check the labels
        let domain_conf = this.xref_conf[result.domain_id];

        if (domain_conf.trim_after) {
            result.url = domain_conf.url.replace("£{id}", encodeURIComponent(result.identifier.substring(0, result.identifier.indexOf(domain_conf.trim_after))));
        } else {
            result.url = domain_conf.url.replace("£{id}", encodeURIComponent(result.identifier));
        }

        this.preparePaging(result);

        this.prepareFilter(result);

        this.applyGlobFilter(result);
    }

    prepareFilter(result) {

        let domain_counts = result.domain_counts;
        //first sort by count
        domain_counts.sort(function (a, b) {
            if (a.count < b.count) return 1;
            if (a.count > b.count) return -1;
            return 0;
        });
        for (let key3 in domain_counts) {
            let domain_count = domain_counts[key3];
            domain_count.selected = true;
            try {
                domain_count.filterLabel = this.xref_conf[domain_count.domain_id].name + '(' + domain_count.count.toLocaleString() + ')';
            } catch (e) {
                domain_count.filterLabel = domain_count.domain_id;
            }
        }

    }

    preparePaging(result) {

        result.clientPage = 0;
        result.maxClientPage = 0;
        result.serverPage = 0;
        result.maxServerPage = 0;

        if (result.hasFilter) { //if filter active total count is equal to selected ones
            let filter_total = 0;
            for (let key in result.domain_counts) {
                let domain_count = result.domain_counts[key];
                if (domain_count.selected) {
                    filter_total += domain_count.count;
                }
            }
            result.count = filter_total;
        }

        if (result.count > this.app_conf.page_size) {

            result.maxClientPage = Math.ceil(result.count / this.app_conf.page_size) - 1;

            if (result.count > this.app_conf.server_result_page_size) {
                result.maxServerPage = Math.ceil(result.count / this.app_conf.server_result_page_size);
            }

            result.displayEntries = result.entries.slice(0, this.app_conf.page_size);

        } else {

            result.displayEntries = result.entries;

        }

        this.prepareEntries(result);
    }

    prepareEntries(result) {

        for (let key in result.entries) {

            let entry = result.entries[key];

            let domain_conf = this.xref_conf[entry.domain_id];
            if (domain_conf.trim_after) {
                entry.url = domain_conf.url.replace("£{id}", encodeURIComponent(entry.xref_id.substring(0, entry.xref_id.indexOf(domain_conf.trim_after))));
            } else {
                entry.url = domain_conf.url.replace("£{id}", encodeURIComponent(entry.xref_id));
            }

            if (entry.xref_id.length <= 12) {
                entry.label = entry.xref_id;
                entry.title = '';
            } else {
                entry.label = entry.xref_id.substring(0, 10) + '...';
                entry.title = entry.xref_id;
            }

            entry.style = {
                'background-color': this.app_conf.box_color
            }
        }

    }

    applyGlobFilter(result) {

        if (this.hasGlobalFilter) {

            let datasets = this.app_conf.global_filter_datasets;
            let domain_counts = result.domain_counts;
            //first unselect all 
            for (let key in domain_counts) {
                domain_counts[key].selected = false;
            }

            let found = false;
            for (let index = 0; index < datasets.length; index++) {
                const element = datasets[index];
                for (let key2 in domain_counts) {
                    if (domain_counts[key2].domain_id === element.id) {
                        found = true;
                        domain_counts[key2].selected = true;
                    }
                }
            }
            if (!found) {

                //TODO set displayed entries empty
                if (result.depth == 0) {
                    this.addResult(result);
                }
                result.count = 0;
                result.displayEntries = [];

            } else {

                let filters = '';
                for (var key in domain_counts) {
                    let domain_count = domain_counts[key];
                    if (domain_count.selected) {
                        filters += domain_count.domain_id + ',';
                    }
                }

                result.filters = filters;
                result.hasFilter = true;
                result.lastFilteredPageKey = null;

                this.fetcher.searchByFilter(
                    result,
                    this.processGlobalFilteredResults.bind(this),
                    []
                );

            }

        }

    }

    processSelectedXref(results, callback_params) {

        for (let key in results) {
            let result = results[key];
            for (let key2 in result.results) {
                let sub_result = result.results[key2];
                if (sub_result.identifier === callback_params[2] && sub_result.domain_id === callback_params[3]) {

                    // now add this result to the selected Xrefs
                    this.prepareResult(sub_result, result, callback_params[4].depth + 1);
                    callback_params[4].selectedXrefs.unshift(sub_result);
                    callback_params[5].selected = true;
                    callback_params[5].style["background-color"] = this.app_conf.selected_box_color;

                    return sub_result;
                }
            }
        }

    }

    processGlobalFilteredResults(data_results, result, fromPaging) {

        this.processFilteredResults(data_results, result, false);

        if (result.depth == 0) {
            this.addResult(result);
        }

    }
    processFilteredResults(data_results, result, fromPaging) {


        this.prepareEntries(data_results[0].results[0]);

        if (fromPaging) {
            Array.prototype.push.apply(result.entries, data_results[0].results[0].entries);
        } else {
            result.entries = data_results[0].results[0].entries;
            this.preparePaging(result);
        }

        result.lastFilteredPageKey = data_results[0].results[0].identifier; //this is a bit magic for now. we are getting pageindex from identifer field
    }

    processPagingResults(results, result_org) {

        result_org.serverPage++;

        for (let key in results) {
            let result = results[key];
            for (let key2 in result.results) {
                let sub_result = result.results[key2];
                if (sub_result.identifier === result_org.identifier && sub_result.domain_id === result_org.domain_id) {
                    // now add all the result entries to existing entries
                    //eclipse issue
                    //sub_result_org.entries.push(...sub_result.entries);
                    this.prepareEntries(sub_result);
                    Array.prototype.push.apply(result_org.entries, sub_result.entries);
                }
            }
        }

    }

    resetPaging() {

        for (let key in this.all_sub_results) {
            this.preparePaging(this.all_sub_results[key]);
        }

    }

    resetBoxColors() {

        for (let key in this.all_sub_results) {
            let sub_result = this.all_sub_results[key];

            changeColors(sub_result, this.app_conf);

            change_all_sub_entries(sub_result, this.app_conf);

        }

        function change_all_sub_entries(sub_result, app_conf) {

            for (let key in sub_result.selectedXrefs) {
                let sel_sub_result = sub_result.selectedXrefs[key];
                changeColors(sel_sub_result, app_conf);
                change_all_sub_entries(sel_sub_result, app_conf);
            }
        }

        function changeColors(sub_result, app_conf) {

            for (let key3 in sub_result.entries) {
                let entry = sub_result.entries[key3];
                if (entry.selected) {
                    entry.style["background-color"] = app_conf.selected_box_color;
                } else {
                    entry.style["background-color"] = app_conf.box_color;
                }
            }
        }

    }

}